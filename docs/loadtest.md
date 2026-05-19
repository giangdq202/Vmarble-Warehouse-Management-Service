# Load Testing the Warehouse Server

A local Go-native load tester lives at `cmd/loadtest`. It drives a configurable
mix of HTTP scenarios against a running `warehouse-server` and writes a
self-contained HTML report (plus `summary.json`) to `loadtest-results/`.

The tool is intentionally local-only: no metrics shipping, no shared backends.
Every artifact a teammate needs to interpret a run is in the same folder.

## Why a custom tool

`vegeta`, `k6`, and `pgbench` are excellent. We built our own because:

- The repo is already Go. Payload factories, auth helpers, and DTOs are reused
  directly — no copy-pasting schemas into a JS or Lua file.
- Mixing HTTP-level scenarios with future direct-`pgxpool` scenarios (e.g.
  seed-then-read benchmarks) is a Go-native concern.
- Closed-loop and open-loop semantics are explicit knobs, so coordinated
  omission cannot quietly understate P99.

## Quick start

```bash
# Terminal 1: backend
make dev

# Terminal 2: 60s smoke run, mixed read scenario, 16 VUs
make loadtest

# Open the report (path is in the final log line)
open loadtest-results/<run>/report.html
```

You can also run the binary directly for fine-grained control:

```bash
go build -o bin/loadtest ./cmd/loadtest
./bin/loadtest \
  -scenario=list-pos \
  -mode=open -rps=300 \
  -duration=2m -vus=64 \
  -base-url=http://localhost:8080 \
  -jwt-secret="$JWT_SECRET" \
  -p95-ms=250 -err-rate-pct=1 -exit-on-fail
```

## Scenarios

| Name | What it does | When to use |
|------|--------------|-------------|
| `healthz` | Spam `GET /healthz` | Baseline — isolates HTTP/transport overhead |
| `list-pos` | Paginate `GET /api/v1/pos` | Hot read on `material_purchase_orders` |
| `list-work-orders` | Paginate `GET /api/v1/work-orders` | Hot read on `work_orders` |
| `list-skus` | Paginate `GET /api/v1/skus` | Hot read on `skus` (catalog browsing) |
| `mixed` | Weighted round-robin of the above | Closest approximation to real read traffic |

Add new scenarios under `internal/loadtest/scenarios/` (one file per
scenario), then expose them via the switch in `cmd/loadtest/main.go`.

## Modes: closed-loop vs open-loop

**Closed-loop** (`-mode=closed`, default): each VU loops `Step → record →
Step`. Throughput equals whatever the system can give you at the chosen
concurrency. Best for "max sustained RPS at N concurrent users" questions.

**Open-loop** (`-mode=open -rps=300`): a token-bucket limiter dispatches one
request per tick regardless of how many VUs are blocked. Necessary for
honest P99 numbers — closed-loop hides queueing delay because slow requests
stall their VU and prevent further dispatches. See Coda Hale's *"Your Load
Generator Is Probably Lying About The Latencies"*.

If `mixed-realistic`-style numbers matter for capacity planning, run
**open-loop**. If you only care about peak throughput, **closed-loop** is
fine.

## Metrics

The recorder uses an HDR histogram (`HdrHistogram/hdrhistogram-go`) bounded
to 1µs..60s with 3 significant figures (~0.1ms accuracy at 100ms,
~10ms at 10s). Memory is fixed at ~38 KB per histogram regardless of sample
count, which is what makes a 5-minute test on a laptop sensible.

The report shows:

- Global summary: total / 2xx / 4xx / 5xx / errs, mean, P50, P90, P95, P99,
  P99.9, max.
- Per-endpoint table with the same percentile bundle.
- Time-series chart of RPS, errors-per-second, P50, P95, P99 in 1-second
  buckets.
- Cumulative latency CDF on a log-x axis (so the long tail is readable).
- System info (host, OS, arch, CPU count, Go version) so a saved report
  remains interpretable months later.

`summary.json` is the same data structurally, suitable for diffing two runs
or feeding into a regression script.

## Thresholds (CI gates)

Pass `-p95-ms`, `-p99-ms`, `-err-rate-pct` to assert SLOs. Each one shows up
as a green/red row at the top of the report. Add `-exit-on-fail` to make
the binary exit non-zero on any failure — the regular CI check pattern.

```bash
./bin/loadtest -scenario=mixed -duration=2m -vus=32 \
  -p95-ms=200 -p99-ms=500 -err-rate-pct=0.5 -exit-on-fail
```

A failed threshold does **not** abort the run; the report and JSON are
still written so you can diagnose. Only `-exit-on-fail` controls the
process exit code.

## Recommended workflow

1. **Smoke** (`make loadtest`) before merging anything that touches a hot
   read path. 60 seconds of `mixed` against your local DB is enough to
   catch obvious regressions.
2. **Soak** (`make loadtest-soak`) once a sprint or before a release. 15
   minutes catches connection pool leaks and goroutine growth that a smoke
   run misses.
3. **Threshold gate** in CI on the smoke run only. Soak tests are too
   variable for a hard gate; treat them as a dashboard, not a binary check.

## Reading a report

Common patterns and what they mean:

- **P95 stable, P99 climbing over time**: connection pool exhaustion or GC
  pressure. Watch for `pool: max_conns` in server logs.
- **Errors-per-second spike but RPS unchanged**: a downstream dependency
  (e.g. Postgres) is briefly unavailable. Check `pg_stat_activity` and
  Docker logs.
- **CDF has a hard wall at 60s**: the histogram clamp triggered, meaning
  the upstream timeout is likely the same. Inspect server config.
- **Open-loop "achieved RPS << target RPS"**: the worker pool is saturated.
  Either the server is the bottleneck (look at backend P99) or `-vus` is
  set too low for the pacing rate.

## Caveats

- This is a **stress** tool, not a chaos tool: there is no failure
  injection, latency simulation, or network partitioning. Layer those in
  separately if you need them.
- The HDR clamp at 60s means absurd outliers (DNS hangs, OS-level pauses)
  show up at the wall. That's a feature for percentile math, but read the
  per-second timeline if a single 60s spike matters to you.
- The mock token (`-jwt-secret`) bypasses real auth flows. If you want to
  exercise the JWT path itself, mint tokens externally and pass `-token`.
