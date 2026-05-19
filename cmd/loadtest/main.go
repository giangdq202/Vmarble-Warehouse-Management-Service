// Command loadtest drives a configurable mix of HTTP scenarios against a
// running warehouse-server instance and writes a self-contained HTML
// report (plus summary.json) to a local directory.
//
// Quick start:
//
//	make dev                                  # in another terminal
//	make loadtest                             # 60s smoke test, mixed scenario
//	open loadtest-results/<latest>/report.html
//
// The tool is intentionally local-only: no metrics shipping, no shared
// state. Reports are byte-for-byte reproducible from summary.json so they
// travel well in PR descriptions and incident threads.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
	"github.com/vmarble/warehouse-management-service/internal/loadtest/report"
	"github.com/vmarble/warehouse-management-service/internal/loadtest/scenarios"
	"github.com/vmarble/warehouse-management-service/internal/loadtest/sysinfo"
	"github.com/vmarble/warehouse-management-service/internal/platform/auth"
)

func main() {
	var (
		scenarioName = flag.String("scenario", "mixed", "scenario name: healthz | list-pos | list-work-orders | list-skus | mixed")
		mode         = flag.String("mode", "closed", "request pacing: closed | open")
		duration     = flag.Duration("duration", 60*time.Second, "wall-clock budget for the test")
		warmup       = flag.Duration("warmup", 0, "warmup period before stats collection starts (results during warmup are still recorded)")
		vus          = flag.Int("vus", 16, "virtual users (closed-loop concurrency / open-loop dispatcher pool)")
		rps          = flag.Float64("rps", 0, "open-loop target rps (ignored in closed-loop)")
		baseURL      = flag.String("base-url", "http://localhost:8080", "warehouse-server base URL")
		token        = flag.String("token", "", "Bearer token; if empty and -jwt-secret is set, a short-lived admin token is minted")
		jwtSecret    = flag.String("jwt-secret", "", "shared secret used to mint a token when -token is empty")
		outDir       = flag.String("out-dir", "loadtest-results", "directory where the run subfolder is written")
		// Threshold knobs — surfaced as flags so the same binary can run on
		// laptop dev (loose) or CI (strict) without recompiling.
		p95Threshold      = flag.Float64("p95-ms", 0, "fail report if global P95 (ms) is above this; 0 disables the check")
		p99Threshold      = flag.Float64("p99-ms", 0, "fail report if global P99 (ms) is above this; 0 disables the check")
		errRateThreshold  = flag.Float64("err-rate-pct", 0, "fail report if (5xx + transport-err) / total exceeds this percentage; 0 disables the check")
		exitOnFail        = flag.Bool("exit-on-fail", false, "exit with non-zero status if any threshold check fails (useful in CI)")
	)
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Mint a token if needed. The auth package is the single source of truth
	// for token format, so we don't reimplement it here.
	bearer := *token
	if bearer == "" && *jwtSecret != "" {
		bearer = auth.SignToken(*jwtSecret, "loadtest", auth.RoleAdmin, time.Now().Add(2*time.Hour))
		logger.Info("minted admin token for load test (2h ttl)")
	}

	client := &loadtest.Client{
		HTTP: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				// Bumping these matters: Go's defaults (2 + 100) starve
				// quickly at vus=200+ and the test ends up measuring DNS
				// lookups instead of server work.
				MaxIdleConns:        4096,
				MaxIdleConnsPerHost: 4096,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		BaseURL: strings.TrimRight(*baseURL, "/"),
		Token:   bearer,
	}

	scenario, err := buildScenario(*scenarioName, client)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}

	// SIGINT lets the operator hit Ctrl-C and still get a partial report.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	startedAt := time.Now()
	rec := loadtest.NewRecorder(startedAt)

	logger.Info("loadtest starting",
		"scenario", scenario.Name(),
		"mode", *mode,
		"vus", *vus,
		"rps", *rps,
		"duration", *duration,
		"base_url", client.BaseURL,
	)
	cfg := loadtest.RunConfig{
		Mode:     loadtest.Mode(*mode),
		VUs:      *vus,
		RPS:      *rps,
		Duration: *duration,
		Warmup:   *warmup,
		Scenario: scenario,
		Recorder: rec,
		Logger:   logger,
	}
	if err := loadtest.Run(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, "loadtest:", err)
		os.Exit(2)
	}

	endedAt := time.Now()
	snap := rec.Snapshot(endedAt)

	thresholds := report.NewThresholds()
	if *p95Threshold > 0 {
		thresholds.P95Under(*p95Threshold)
	}
	if *p99Threshold > 0 {
		thresholds.P99Under(*p99Threshold)
	}
	if *errRateThreshold > 0 {
		thresholds.ErrorRateUnder(*errRateThreshold)
	}
	results := thresholds.Evaluate(snap)

	host := sysinfo.Capture(ctx, client.BaseURL)

	bundle := report.Bundle{
		GeneratedAt: time.Now(),
		Scenario:    scenario.Name(),
		Mode:        *mode,
		VUs:         *vus,
		RPSTarget:   *rps,
		DurationS:   duration.Seconds(),
		Host:        host,
		Snapshot:    snap,
		Thresholds:  results,
	}
	dir, err := report.Write(*outDir, bundle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report:", err)
		os.Exit(2)
	}

	logger.Info("loadtest complete",
		"total", snap.TotalReqs,
		"errs", snap.TotalErrs,
		"p50", snap.Global.P50,
		"p95", snap.Global.P95,
		"p99", snap.Global.P99,
		"report_dir", dir,
	)

	failed := false
	for _, r := range results {
		if !r.Passed {
			fmt.Fprintf(os.Stderr, "threshold FAIL: %s want %s got %s\n", r.Name, r.Want, r.Got)
			failed = true
		}
	}
	if failed && *exitOnFail {
		os.Exit(1)
	}
}

func buildScenario(name string, c *loadtest.Client) (loadtest.Scenario, error) {
	switch name {
	case "healthz":
		return &scenarios.Healthz{Client: c}, nil
	case "list-pos":
		return &scenarios.ListPaged{ScenarioName: "list-pos", Path: "/api/v1/pos", Limit: 50, Client: c}, nil
	case "list-work-orders":
		return &scenarios.ListPaged{ScenarioName: "list-work-orders", Path: "/api/v1/work-orders", Limit: 50, Client: c}, nil
	case "list-skus":
		return &scenarios.ListPaged{ScenarioName: "list-skus", Path: "/api/v1/skus", Limit: 50, Client: c}, nil
	case "mixed":
		// Realistic-ish read mix: warehouse staff browse SKUs and POs much
		// more often than they crunch work orders. Tune as your real
		// traffic patterns come into focus.
		return &scenarios.Mixed{Children: []scenarios.WeightedScenario{
			{Scenario: &scenarios.Healthz{Client: c}, Weight: 1},
			{Scenario: &scenarios.ListPaged{ScenarioName: "list-skus", Path: "/api/v1/skus", Limit: 50, Client: c}, Weight: 4},
			{Scenario: &scenarios.ListPaged{ScenarioName: "list-pos", Path: "/api/v1/pos", Limit: 50, Client: c}, Weight: 3},
			{Scenario: &scenarios.ListPaged{ScenarioName: "list-work-orders", Path: "/api/v1/work-orders", Limit: 50, Client: c}, Weight: 2},
		}}, nil
	default:
		return nil, fmt.Errorf("unknown scenario %q (try: healthz, list-pos, list-work-orders, list-skus, mixed)", name)
	}
}
