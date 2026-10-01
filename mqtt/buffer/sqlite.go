// Package buffer provides a durable SQLite-backed offline queue for telemetry.
// Data is written here before any MQTT interaction — nothing is lost on disconnect.
package buffer

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ekicimustafa/industrial-gateway/connector"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS telemetry_buffer (
    id        INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id TEXT    NOT NULL,
    ts        INTEGER NOT NULL,
    values_json TEXT  NOT NULL,
    UNIQUE(device_id, ts)
);
CREATE INDEX IF NOT EXISTS idx_telemetry_buffer_id ON telemetry_buffer(id);
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=5000;
`

// Row is one flushed record: device → timestamp → merged key/value map.
type Row struct {
	ID       int64
	DeviceID string
	Ts       int64
	Values   map[string]any
}

// Buffer is a durable write-ahead queue backed by SQLite.
type Buffer struct {
	db   *sql.DB
	mu   sync.Mutex
	path string
}

// Open opens (or creates) the buffer database at the given path.
func Open(path string) (*Buffer, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("buffer: mkdir %s: %w", filepath.Dir(path), err)
	}
	db, err := sql.Open("sqlite", path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("buffer: open %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("buffer: schema: %w", err)
	}
	return &Buffer{db: db, path: path}, nil
}

// Write persists a batch of DataPoints. Same-device same-timestamp rows are
// merged (keys are unioned), matching the Python gateway behaviour.
func (b *Buffer) Write(points []connector.DataPoint) error {
	if len(points) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	for _, p := range points {
		var existing string
		err := tx.QueryRow(
			`SELECT values_json FROM telemetry_buffer WHERE device_id=? AND ts=?`,
			p.DeviceID, p.Ts,
		).Scan(&existing)

		merged := map[string]any{}
		if err == nil {
			_ = json.Unmarshal([]byte(existing), &merged)
		}
		merged[p.Key] = p.Value

		encoded, _ := json.Marshal(merged)

		_, err = tx.Exec(`
			INSERT INTO telemetry_buffer(device_id, ts, values_json) VALUES(?,?,?)
			ON CONFLICT(device_id, ts) DO UPDATE SET values_json=excluded.values_json`,
			p.DeviceID, p.Ts, string(encoded),
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ReadBatch returns up to n rows in insertion order.
func (b *Buffer) ReadBatch(n int) ([]Row, error) {
	rows, err := b.db.Query(
		`SELECT id, device_id, ts, values_json FROM telemetry_buffer ORDER BY id LIMIT ?`, n,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Row
	for rows.Next() {
		var r Row
		var raw string
		if err := rows.Scan(&r.ID, &r.DeviceID, &r.Ts, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(raw), &r.Values)
		result = append(result, r)
	}
	return result, rows.Err()
}

// Delete removes rows by ID after successful publish.
func (b *Buffer) Delete(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	// Build placeholders: DELETE WHERE id IN (?,?,?)
	query := `DELETE FROM telemetry_buffer WHERE id IN (`
	args := make([]any, len(ids))
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += "?"
		args[i] = id
	}
	query += ")"
	_, err := b.db.Exec(query, args...)
	return err
}

// Count returns the number of buffered rows.
func (b *Buffer) Count() (int64, error) {
	var n int64
	err := b.db.QueryRow(`SELECT COUNT(*) FROM telemetry_buffer`).Scan(&n)
	return n, err
}

// Trim removes the oldest rows, keeping at most maxRows total.
func (b *Buffer) Trim(maxRows int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.db.Exec(`
		DELETE FROM telemetry_buffer WHERE id IN (
			SELECT id FROM telemetry_buffer ORDER BY id ASC
			LIMIT MAX(0, (SELECT COUNT(*) FROM telemetry_buffer) - ?)
		)`, maxRows)
	return err
}

// Close closes the underlying database.
func (b *Buffer) Close() error {
	return b.db.Close()
}
