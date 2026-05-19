package loadtest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Mode chooses the request pacing strategy.
//
//   - ModeClosedLoop: each VU loops Step → record → Step. Throughput is
//     whatever the system allows. Useful for "max sustained RPS at a given
//     concurrency" questions.
//   - ModeOpenLoop:   the runner emits one request per tick of a token-bucket
//     limiter, regardless of how many VUs are blocked. Necessary for
//     coordinated-omission-free P99 — under closed-loop, a slow request
//     stalls its VU and "hides" the queue that would have built up.
type Mode string

const (
	ModeClosedLoop Mode = "closed"
	ModeOpenLoop   Mode = "open"
)

// RunConfig is everything the runner needs. Validated in Run.
type RunConfig struct {
	Mode     Mode
	VUs      int           // worker count (closed-loop concurrency, open-loop dispatch parallelism)
	RPS      float64       // open-loop only; ignored in closed-loop
	Duration time.Duration // total wall-clock budget; runner stops at start+Duration
	Warmup   time.Duration // pre-roll time whose results are recorded but flagged in the report (currently just delays start)
	Scenario Scenario
	Recorder *Recorder
	Logger   *slog.Logger // optional; defaults to slog.Default
}

func (c RunConfig) validate() error {
	if c.Scenario == nil {
		return errors.New("loadtest: scenario is required")
	}
	if c.Recorder == nil {
		return errors.New("loadtest: recorder is required")
	}
	if c.Duration <= 0 {
		return errors.New("loadtest: duration must be > 0")
	}
	if c.VUs <= 0 {
		return errors.New("loadtest: vus must be > 0")
	}
	switch c.Mode {
	case ModeClosedLoop:
		// nothing else required
	case ModeOpenLoop:
		if c.RPS <= 0 {
			return errors.New("loadtest: open-loop mode requires rps > 0")
		}
	default:
		return fmt.Errorf("loadtest: unknown mode %q", c.Mode)
	}
	return nil
}

// Run drives the scenario until ctx cancels or Duration elapses, recording
// every Result into cfg.Recorder. Run blocks until all VUs have returned;
// it never leaks goroutines past return.
func Run(ctx context.Context, cfg RunConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Single deadline drives both branches. Using context.WithTimeout means
	// any inflight request honours the same wall-clock as the dispatcher.
	runCtx, cancel := context.WithTimeout(ctx, cfg.Duration+cfg.Warmup)
	defer cancel()

	if cfg.Warmup > 0 {
		logger.Info("warmup", "duration", cfg.Warmup)
		select {
		case <-time.After(cfg.Warmup):
		case <-runCtx.Done():
			return runCtx.Err()
		}
	}

	// Periodic progress log so an operator running `make loadtest` sees the
	// thing is alive without tailing the report file.
	progressDone := make(chan struct{})
	go progressLogger(runCtx, cfg.Recorder, logger, progressDone)
	defer func() { <-progressDone }()

	switch cfg.Mode {
	case ModeClosedLoop:
		runClosedLoop(runCtx, cfg)
	case ModeOpenLoop:
		runOpenLoop(runCtx, cfg)
	}
	return nil
}

// runClosedLoop spins up cfg.VUs goroutines, each calling Step in a tight
// loop until ctx cancels. Throughput = sum of (1 / per-vu latency).
func runClosedLoop(ctx context.Context, cfg RunConfig) {
	var wg sync.WaitGroup
	wg.Add(cfg.VUs)
	for i := 0; i < cfg.VUs; i++ {
		go func(vu int) {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				res := cfg.Scenario.Step(ctx, vu)
				cfg.Recorder.Record(res, time.Now())
			}
		}(i)
	}
	wg.Wait()
}

// runOpenLoop drives a token-bucket at cfg.RPS, dispatching each granted
// token to a worker pool. The pool is bounded at cfg.VUs so we cannot
// runaway-allocate goroutines under a slow backend; if all VUs are busy
// when a token fires, the limiter waits and the actual issued RPS drops —
// the report flags this as "rps achieved < rps target".
func runOpenLoop(ctx context.Context, cfg RunConfig) {
	limiter := rate.NewLimiter(rate.Limit(cfg.RPS), max(int(cfg.RPS), 1))

	jobs := make(chan int, cfg.VUs)
	var wg sync.WaitGroup
	wg.Add(cfg.VUs)
	for i := 0; i < cfg.VUs; i++ {
		go func() {
			defer wg.Done()
			for vu := range jobs {
				res := cfg.Scenario.Step(ctx, vu)
				cfg.Recorder.Record(res, time.Now())
			}
		}()
	}

	vu := 0
	for {
		if err := limiter.Wait(ctx); err != nil {
			break
		}
		select {
		case jobs <- vu:
			vu++
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		}
	}
	close(jobs)
	wg.Wait()
}

func progressLogger(ctx context.Context, rec *Recorder, logger *slog.Logger, done chan<- struct{}) {
	defer close(done)
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	var lastTotal int64
	last := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			total, errs, ok := rec.LiveCounters()
			delta := total - lastTotal
			elapsed := now.Sub(last).Seconds()
			rps := float64(delta) / elapsed
			logger.Info("loadtest progress",
				"total", total, "ok", ok, "errs", errs,
				"rps_recent", fmt.Sprintf("%.1f", rps),
			)
			lastTotal = total
			last = now
		}
	}
}
