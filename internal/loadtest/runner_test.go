package loadtest

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// stubScenario records its own call count and sleeps a fixed amount per
// step. Lets us drive the runner deterministically.
type stubScenario struct {
	name     string
	sleep    time.Duration
	calls    atomic.Int64
	failOnce atomic.Bool
}

func (s *stubScenario) Name() string { return s.name }
func (s *stubScenario) Step(ctx context.Context, _ int) Result {
	s.calls.Add(1)
	select {
	case <-time.After(s.sleep):
	case <-ctx.Done():
		return Result{Endpoint: s.name, Latency: 0}
	}
	if s.failOnce.CompareAndSwap(true, false) {
		return Result{Endpoint: s.name, StatusCode: 500, Latency: s.sleep}
	}
	return Result{Endpoint: s.name, StatusCode: 200, Latency: s.sleep}
}

func TestRun_ClosedLoop_RespectsDuration(t *testing.T) {
	s := &stubScenario{name: "stub", sleep: 5 * time.Millisecond}
	rec := NewRecorder(time.Now())
	start := time.Now()
	err := Run(context.Background(), RunConfig{
		Mode:     ModeClosedLoop,
		VUs:      4,
		Duration: 200 * time.Millisecond,
		Scenario: s,
		Recorder: rec,
	})
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 200*time.Millisecond || elapsed > 800*time.Millisecond {
		t.Errorf("elapsed %v outside [200ms, 800ms]", elapsed)
	}
	total, _, _ := rec.LiveCounters()
	if total < 4 {
		t.Errorf("expected at least one round per VU, got %d", total)
	}
}

func TestRun_OpenLoop_HitsApproximateRPS(t *testing.T) {
	s := &stubScenario{name: "stub", sleep: 1 * time.Millisecond}
	rec := NewRecorder(time.Now())
	err := Run(context.Background(), RunConfig{
		Mode:     ModeOpenLoop,
		VUs:      8,
		RPS:      100,
		Duration: 500 * time.Millisecond,
		Scenario: s,
		Recorder: rec,
	})
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	total, _, _ := rec.LiveCounters()
	// Token-bucket warmup + initial burst (== rate) means we don't hit
	// exactly 50. Accept a wide band; we're testing that pacing engages
	// at all, not a specific number.
	if total < 20 || total > 200 {
		t.Errorf("open-loop @100rps for 0.5s yielded %d requests, want 20..200", total)
	}
}

func TestRun_RejectsBadConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  RunConfig
	}{
		{"missing scenario", RunConfig{Mode: ModeClosedLoop, VUs: 1, Duration: time.Second, Recorder: NewRecorder(time.Now())}},
		{"missing recorder", RunConfig{Mode: ModeClosedLoop, VUs: 1, Duration: time.Second, Scenario: &stubScenario{}}},
		{"zero duration", RunConfig{Mode: ModeClosedLoop, VUs: 1, Duration: 0, Scenario: &stubScenario{}, Recorder: NewRecorder(time.Now())}},
		{"zero vus", RunConfig{Mode: ModeClosedLoop, VUs: 0, Duration: time.Second, Scenario: &stubScenario{}, Recorder: NewRecorder(time.Now())}},
		{"open-loop no rps", RunConfig{Mode: ModeOpenLoop, VUs: 1, Duration: time.Second, Scenario: &stubScenario{}, Recorder: NewRecorder(time.Now())}},
		{"unknown mode", RunConfig{Mode: "weird", VUs: 1, Duration: time.Second, Scenario: &stubScenario{}, Recorder: NewRecorder(time.Now())}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Run(context.Background(), tc.cfg); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestRun_HonoursContextCancel(t *testing.T) {
	s := &stubScenario{name: "stub", sleep: 10 * time.Millisecond}
	rec := NewRecorder(time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	if err := Run(ctx, RunConfig{
		Mode: ModeClosedLoop, VUs: 2, Duration: 5 * time.Second,
		Scenario: s, Recorder: rec,
	}); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	elapsed := time.Since(start)
	if elapsed > 1*time.Second {
		t.Errorf("ctx cancel did not stop run promptly: elapsed=%v", elapsed)
	}
}
