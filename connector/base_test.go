package connector

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProto is a scriptable Protocol.
type fakeProto struct {
	failConnects int32 // Connect fails this many times before succeeding
	panicRuns    int32 // Run panics this many times before blocking
	hangOnClose  bool  // Disconnect ignores its context and never returns

	connects, runs, disconnects atomic.Int32
}

func (f *fakeProto) Connect(context.Context) error {
	if f.connects.Add(1) <= f.failConnects {
		return errors.New("device unreachable")
	}
	return nil
}

func (f *fakeProto) Run(ctx context.Context) error {
	if f.runs.Add(1) <= f.panicRuns {
		panic("bad register map")
	}
	<-ctx.Done()
	return nil
}

func (f *fakeProto) Disconnect(context.Context) error {
	f.disconnects.Add(1)
	if f.hangOnClose {
		select {}
	}
	return nil
}

// newTestBase returns a Base with millisecond delays so tests run fast.
func newTestBase(p Protocol, out chan DataPoint) *Base {
	b := NewBase(Config{ID: "c1", Type: TypeModbusTCP, Name: "test"}, out, p)
	b.breaker.RetryDelay = time.Millisecond
	b.breaker.BaseDelay = time.Millisecond
	b.stopWait = time.Second
	b.disconnectTimeout = 50 * time.Millisecond
	return b
}

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBaseReconnectsUntilActive(t *testing.T) {
	p := &fakeProto{failConnects: 7} // crosses the default threshold of 5
	b := newTestBase(p, make(chan DataPoint, 1))

	b.Start(context.Background())
	waitFor(t, "active status", func() bool { return b.Status() == StatusActive })

	if got := p.connects.Load(); got != 8 {
		t.Errorf("connects = %d, want 8", got)
	}
	if got := b.breaker.Streak(); got != 0 {
		t.Errorf("breaker streak after success = %d, want 0", got)
	}

	b.Stop()
	if b.Running() || b.Status() != StatusStopped {
		t.Errorf("after Stop: running=%v status=%s", b.Running(), b.Status())
	}
	if got := p.disconnects.Load(); got != 1 {
		t.Errorf("disconnects = %d, want 1", got)
	}
}

func TestBaseRecoversFromPanic(t *testing.T) {
	p := &fakeProto{panicRuns: 2}
	b := newTestBase(p, make(chan DataPoint, 1))

	b.Start(context.Background())
	defer b.Stop()

	waitFor(t, "third run", func() bool { return p.runs.Load() == 3 })
	waitFor(t, "active status", func() bool { return b.Status() == StatusActive })
}

func TestBaseStopsWhenParentCancelled(t *testing.T) {
	b := newTestBase(&fakeProto{}, make(chan DataPoint, 1))
	ctx, cancel := context.WithCancel(context.Background())

	b.Start(ctx)
	waitFor(t, "active status", func() bool { return b.Status() == StatusActive })
	cancel()
	waitFor(t, "loop exit", func() bool { return !b.Running() })
	b.Stop()
}

func TestBaseStopBoundedByHungDisconnect(t *testing.T) {
	b := newTestBase(&fakeProto{hangOnClose: true}, make(chan DataPoint, 1))
	b.Start(context.Background())
	waitFor(t, "active status", func() bool { return b.Status() == StatusActive })

	start := time.Now()
	b.Stop()
	if took := time.Since(start); took > time.Second {
		t.Fatalf("Stop took %v with a hung Disconnect", took)
	}
	if b.Status() != StatusStopped {
		t.Errorf("status = %s, want stopped", b.Status())
	}
}

func TestBaseStartTwiceKeepsOneLoop(t *testing.T) {
	p := &fakeProto{}
	b := newTestBase(p, make(chan DataPoint, 1))

	b.Start(context.Background())
	defer b.Stop()
	waitFor(t, "first run", func() bool { return p.runs.Load() == 1 })

	b.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	if got := p.connects.Load(); got != 1 {
		t.Errorf("connects = %d, want 1 (second Start must be ignored)", got)
	}
}

func TestEmitDropsWhenQueueFull(t *testing.T) {
	out := make(chan DataPoint, 1)
	b := newTestBase(&fakeProto{}, out)

	b.Emit("dev-1", "voltage", 230.0)
	b.EmitAt("dev-1", "current", 5.0, 42) // queue full: must not block

	if len(out) != 1 {
		t.Fatalf("queue len = %d, want 1", len(out))
	}
	dp := <-out
	if dp.ConnectorID != "c1" || dp.DeviceID != "dev-1" || dp.Key != "voltage" || dp.Value != 230.0 || dp.Ts == 0 {
		t.Errorf("unexpected datapoint %+v", dp)
	}
}

func TestDefaultHandlers(t *testing.T) {
	b := newTestBase(&fakeProto{}, make(chan DataPoint, 1))
	ctx := context.Background()

	if res := b.HandleRPC(ctx, "dev-1", RPCRequest{Method: "set"}); res["error"] != "RPC not supported by connector type 'modbus_tcp'" {
		t.Errorf("HandleRPC() = %v", res)
	}
	if res := b.ManagementProbe(ctx, ""); res["ok"] != false || res["connector_id"] != "c1" {
		t.Errorf("ManagementProbe() = %v", res)
	}
	if states := b.DeviceConnectionStates(); states == nil || len(states) != 0 {
		t.Errorf("DeviceConnectionStates() = %v, want empty non-nil map", states)
	}
}
