package buffer_test

import (
	"os"
	"testing"

	"github.com/ekicimustafa/industrial-gateway/connector"
	"github.com/ekicimustafa/industrial-gateway/mqtt/buffer"
)

// openTestBuffer creates a temporary SQLite database for one test.
// t.Cleanup ensures the file is deleted after the test finishes,
// even if the test fails — equivalent to Python's pytest fixture teardown.
func openTestBuffer(t *testing.T) *buffer.Buffer {
	t.Helper()
	f, err := os.CreateTemp("", "buffer_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	buf, err := buffer.Open(f.Name())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { buf.Close() })
	return buf
}

func TestWrite_and_ReadBatch(t *testing.T) {
	buf := openTestBuffer(t)

	points := []connector.DataPoint{
		{DeviceID: "dev-1", Key: "voltage", Value: 220.5, Ts: 1000},
		{DeviceID: "dev-1", Key: "current", Value: 10.2, Ts: 1000},
		{DeviceID: "dev-2", Key: "power", Value: 500.0, Ts: 2000},
	}

	if err := buf.Write(points); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// dev-1 ts=1000 and dev-2 ts=2000 → 2 rows (same device+ts merges)
	rows, err := buf.ReadBatch(10)
	if err != nil {
		t.Fatalf("ReadBatch: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}

	// dev-1 row should have both keys merged
	dev1 := rows[0]
	if dev1.DeviceID != "dev-1" {
		t.Errorf("want dev-1, got %s", dev1.DeviceID)
	}
	if dev1.Values["voltage"] == nil {
		t.Error("want voltage key in merged row")
	}
	if dev1.Values["current"] == nil {
		t.Error("want current key in merged row")
	}
}

func TestWrite_merge_same_device_ts(t *testing.T) {
	buf := openTestBuffer(t)

	// Two separate Write calls for the same device+ts — should merge, not duplicate
	if err := buf.Write([]connector.DataPoint{
		{DeviceID: "dev-1", Key: "a", Value: 1.0, Ts: 9999},
	}); err != nil {
		t.Fatal(err)
	}
	if err := buf.Write([]connector.DataPoint{
		{DeviceID: "dev-1", Key: "b", Value: 2.0, Ts: 9999},
	}); err != nil {
		t.Fatal(err)
	}

	rows, _ := buf.ReadBatch(10)
	if len(rows) != 1 {
		t.Fatalf("want 1 merged row, got %d", len(rows))
	}
	if rows[0].Values["a"] == nil || rows[0].Values["b"] == nil {
		t.Error("both keys should be present in the merged row")
	}
}

func TestDelete(t *testing.T) {
	buf := openTestBuffer(t)

	buf.Write([]connector.DataPoint{ //nolint:errcheck
		{DeviceID: "dev-1", Key: "x", Value: 1, Ts: 1},
		{DeviceID: "dev-2", Key: "x", Value: 2, Ts: 2},
	})

	rows, _ := buf.ReadBatch(10)
	if len(rows) != 2 {
		t.Fatalf("setup: want 2 rows, got %d", len(rows))
	}

	// Delete only the first row
	if err := buf.Delete([]int64{rows[0].ID}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	remaining, _ := buf.ReadBatch(10)
	if len(remaining) != 1 {
		t.Fatalf("want 1 row after delete, got %d", len(remaining))
	}
	if remaining[0].DeviceID != "dev-2" {
		t.Errorf("wrong row survived: %s", remaining[0].DeviceID)
	}
}

func TestCount(t *testing.T) {
	buf := openTestBuffer(t)

	n, _ := buf.Count()
	if n != 0 {
		t.Fatalf("empty buffer should have count 0, got %d", n)
	}

	buf.Write([]connector.DataPoint{ //nolint:errcheck
		{DeviceID: "d", Key: "k", Value: 1, Ts: 1},
		{DeviceID: "d", Key: "k", Value: 2, Ts: 2},
	})

	n, _ = buf.Count()
	if n != 2 {
		t.Fatalf("want count 2, got %d", n)
	}
}

func TestTrim(t *testing.T) {
	buf := openTestBuffer(t)

	// Write 5 rows
	for i := int64(1); i <= 5; i++ {
		buf.Write([]connector.DataPoint{ //nolint:errcheck
			{DeviceID: "d", Key: "k", Value: float64(i), Ts: i},
		})
	}

	// Trim to 3 — oldest 2 should be removed
	if err := buf.Trim(3); err != nil {
		t.Fatalf("Trim: %v", err)
	}

	n, _ := buf.Count()
	if n != 3 {
		t.Fatalf("want 3 rows after trim, got %d", n)
	}

	// Remaining rows should be the newest (ts 3, 4, 5)
	rows, _ := buf.ReadBatch(10)
	if rows[0].Ts != 3 {
		t.Errorf("want oldest remaining ts=3, got ts=%d", rows[0].Ts)
	}
}

func TestReadBatch_respects_limit(t *testing.T) {
	buf := openTestBuffer(t)

	for i := int64(1); i <= 10; i++ {
		buf.Write([]connector.DataPoint{ //nolint:errcheck
			{DeviceID: "d", Key: "k", Value: float64(i), Ts: i},
		})
	}

	rows, _ := buf.ReadBatch(3)
	if len(rows) != 3 {
		t.Fatalf("want 3 rows with limit=3, got %d", len(rows))
	}
}
