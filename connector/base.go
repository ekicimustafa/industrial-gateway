package connector

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/ekicimustafa/industrial-gateway/internal/env"
)

// Base implements the connector lifecycle shared by every protocol:
// background run loop, reconnect with circuit breaker, bounded stop, and
// non-blocking telemetry emit (Python: the Connector ABC's concrete part).
//
// A concrete connector embeds *Base and passes itself as the Protocol:
//
//	type TCP struct{ *connector.Base; ... }
//
//	func NewTCP(cfg connector.Config, out chan<- connector.DataPoint) *TCP {
//		t := &TCP{}
//		t.Base = connector.NewBase(cfg, out, t)
//		return t
//	}
type Base struct {
	cfg   Config
	out   chan<- DataPoint
	proto Protocol
	log   *slog.Logger

	breaker           *Breaker
	stopWait          time.Duration // how long Stop waits for the loop to exit
	disconnectTimeout time.Duration // how long Stop waits for Disconnect

	mu     sync.Mutex // guards status, cancel, done
	status Status
	cancel context.CancelFunc
	done   chan struct{} // closed when the run loop exits
}

// NewBase wires the shared lifecycle to a protocol implementation.
func NewBase(cfg Config, out chan<- DataPoint, proto Protocol) *Base {
	return &Base{
		cfg:   cfg,
		out:   out,
		proto: proto,
		log:   newLogger(cfg),
		breaker: NewBreaker(
			env.Int("GATEWAY_CB_THRESHOLD", 5),
			env.Seconds("GATEWAY_CB_BASE_DELAY", 30*time.Second),
		),
		stopWait:          5 * time.Second,
		disconnectTimeout: env.Seconds("GATEWAY_CONNECTOR_DISCONNECT_TIMEOUT_SEC", 5*time.Second),
		status:            StatusStopped,
	}
}

func (b *Base) ID() string           { return b.cfg.ID }
func (b *Base) Type() Type           { return b.cfg.Type }
func (b *Base) Name() string         { return b.cfg.Name }
func (b *Base) Config() Config       { return b.cfg }
func (b *Base) Logger() *slog.Logger { return b.log }

func (b *Base) Status() Status {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.status
}

func (b *Base) setStatus(s Status) {
	b.mu.Lock()
	b.status = s
	b.mu.Unlock()
}

// Running reports whether the run loop goroutine is alive.
func (b *Base) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return isOpen(b.done)
}

// Start launches the run loop. Calling it while the loop runs is a no-op.
func (b *Base) Start(ctx context.Context) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if isOpen(b.done) {
		b.log.Warn("already running, ignoring start")
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.done = make(chan struct{})
	b.status = StatusConnecting
	go b.safeRun(runCtx, b.done)
	b.log.Info("started")
}

// Stop cancels the run loop, waits up to stopWait for it to exit, then calls
// Disconnect bounded by disconnectTimeout. It never blocks longer than that.
func (b *Base) Stop() {
	b.mu.Lock()
	cancel, done := b.cancel, b.done
	b.cancel, b.done = nil, nil
	b.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(b.stopWait):
			b.log.Warn("run loop did not exit in time, disconnecting anyway", "waited", b.stopWait)
		}
	}
	b.disconnect()
	b.setStatus(StatusStopped)
	b.log.Info("stopped")
}

// ManagementProbe default: probing is not implemented for this protocol.
func (b *Base) ManagementProbe(context.Context, string) map[string]any {
	return map[string]any{
		"ok":           false,
		"connector_id": b.cfg.ID,
		"type":         b.cfg.Type,
		"error":        fmt.Sprintf("probe not implemented for connector type '%s'", b.cfg.Type),
	}
}

// DeviceConnectionStates default: no per-device tracking.
func (b *Base) DeviceConnectionStates() map[string]string {
	return map[string]string{}
}

// HandleRPC default: this protocol does not accept commands.
func (b *Base) HandleRPC(_ context.Context, deviceName string, _ RPCRequest) map[string]any {
	b.log.Warn("RPC not implemented for this connector type", "device", deviceName)
	return map[string]any{"error": fmt.Sprintf("RPC not supported by connector type '%s'", b.cfg.Type)}
}

// Emit sends a reading stamped with the current time. It never blocks: when
// the gateway queue is full the point is dropped and logged.
func (b *Base) Emit(deviceID, key string, value any) {
	b.EmitAt(deviceID, key, value, time.Now().UnixMilli())
}

// EmitAt is Emit with an explicit Unix-millisecond timestamp.
func (b *Base) EmitAt(deviceID, key string, value any, ts int64) {
	dp := DataPoint{ConnectorID: b.cfg.ID, DeviceID: deviceID, Key: key, Value: value, Ts: ts}
	select {
	case b.out <- dp:
	default:
		b.log.Warn("output queue full, dropping datapoint", "device", deviceID, "key", key)
	}
}

// safeRun is the reconnect loop (Python: _safe_run). It exits only when ctx
// is cancelled; every error or panic from the protocol is retried.
func (b *Base) safeRun(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	defer b.setStatus(StatusStopped)

	b.log.Info("connector loop running",
		"cb_threshold", b.breaker.Threshold, "cb_base_delay", b.breaker.BaseDelay)

	for ctx.Err() == nil {
		if wait := b.breaker.Remaining(); wait > 0 {
			b.log.Debug("circuit breaker open, waiting", "wait", wait)
			sleep(ctx, wait)
			continue
		}

		if err := guard(func() error { return b.proto.Connect(ctx) }); err != nil {
			if ctx.Err() != nil {
				return
			}
			b.fail(ctx, "connection failed", err)
			continue
		}

		if prev := b.breaker.RecordSuccess(); prev > 0 {
			b.log.Info("connection restored, error streak reset", "streak", prev)
		}
		b.setStatus(StatusActive)

		err := guard(func() error { return b.proto.Run(ctx) })
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			b.fail(ctx, "run failed", err)
			continue
		}
		b.log.Warn("run exited unexpectedly, retrying", "in", b.breaker.RetryDelay)
		sleep(ctx, b.breaker.RetryDelay)
	}
}

// fail records an error with the breaker and sleeps the back-off delay.
func (b *Base) fail(ctx context.Context, msg string, err error) {
	b.setStatus(StatusError)
	delay, opened := b.breaker.RecordError()
	if opened {
		b.log.Warn("circuit breaker open",
			"errors", b.breaker.Streak(), "backoff", delay, "until", time.Now().Add(delay).Format(time.TimeOnly))
	}
	b.log.Error(msg, "err", err, "retry_in", delay)
	sleep(ctx, delay)
}

// disconnect runs Disconnect, giving up after disconnectTimeout even if the
// protocol ignores its context.
func (b *Base) disconnect() {
	ctx, cancel := context.WithTimeout(context.Background(), b.disconnectTimeout)
	defer cancel()

	// Buffered so the goroutine can still finish (and be collected) if we
	// stop waiting for it.
	errc := make(chan error, 1)
	go func() { errc <- guard(func() error { return b.proto.Disconnect(ctx) }) }()

	select {
	case err := <-errc:
		if err != nil {
			b.log.Warn("disconnect failed", "err", err)
		}
	case <-ctx.Done():
		b.log.Warn("disconnect timed out, proceeding with best-effort stop", "timeout", b.disconnectTimeout)
	}
}

// guard runs fn and turns a panic into an error, so one faulty connector
// cannot crash the gateway process.
func guard(fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v\n%s", r, debug.Stack())
		}
	}()
	return fn()
}

// sleep waits for d or until ctx is cancelled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// isOpen reports whether done exists and has not been closed yet.
func isOpen(done chan struct{}) bool {
	if done == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
		return true
	}
}
