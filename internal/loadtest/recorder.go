// Package loadtest implements a Go-native HTTP load tester for the warehouse
// API. The tool runs locally (`make loadtest`), drives a configurable mix of
// scenarios against a running backend, and writes a self-contained HTML
// report so you can read percentiles + time-series without an internet
// connection.
//
// Why a custom tool instead of vegeta / k6 / pgbench?
//   - The repo already speaks Go; payload factories can reuse module DTOs and
//     auth helpers without copying schemas into a JS/Lua scenario file.
//   - Mixing HTTP scenarios with direct-pgxpool scenarios (e.g. seed +
//     read-after-write benchmarks) is a Go-native concern.
//   - Closed-loop and open-loop semantics are explicit in the runner so
//     coordinated-omission cannot quietly understate P99 (Coda Hale, "Your
//     Load Generator Is Probably Lying").
//
// See docs/loadtest.md for the full operator guide.
package loadtest

import (
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/HdrHistogram/hdrhistogram-go"
)

// Result is one observation from a Scenario.Step call. The runner constructs
// it after the request completes; recorders fan-out from the single
// per-result struct so we never hold individual samples — only summary
// state plus the HDR histogram.
type Result struct {
	Endpoint   string        // logical name (e.g. "GET /api/v1/work-orders"); empty falls back to scenario name
	StatusCode int           // 0 when the request never reached the server (DNS / connect / context cancel)
	Latency    time.Duration // wall-clock from request issue to response close
	Bytes      int64         // body bytes read (0 if HEAD or empty body)
	Err        error         // non-nil on transport error; status code 0 in that case
}

// Recorder collects observations into HDR histograms and tracks per-endpoint
// counters. The histograms are bounded to [1µs, 60s] with 3 significant
// digits — accurate to ~0.1ms at 100ms, ~10ms at 10s. Memory is fixed at
// ~38KB per histogram regardless of sample count.
//
// All Record methods are safe for concurrent use. The runner calls Record
// from N goroutines without locks blocking the request path; histogram
// internals are protected by a per-recorder mutex held only during the
// record call.
type Recorder struct {
	startedAt time.Time

	mu     sync.Mutex
	global *hdrhistogram.Histogram
	// perEndpoint is keyed by Result.Endpoint. Lazily allocated on first
	// observation so a tool run with one endpoint does not pay for a map of
	// every possible scenario.
	perEndpoint map[string]*endpointStats
	// timeline holds 1-second buckets for the time-series chart in the
	// report. Bucket index = floor(elapsed seconds). Each bucket has its
	// own histogram so the chart can show P95-over-time without
	// interpolating from the cumulative distribution.
	timeline []*timelineBucket

	// total counters are atomics so the periodic "still running" log line
	// in the runner can read them without acquiring mu.
	totalReqs atomic.Int64
	totalErrs atomic.Int64
	total2xx  atomic.Int64
	total4xx  atomic.Int64
	total5xx  atomic.Int64
	totalBody atomic.Int64
}

type endpointStats struct {
	hist  *hdrhistogram.Histogram
	count int64
	errs  int64
	bytes int64
	c2xx  int64
	c4xx  int64
	c5xx  int64
}

type timelineBucket struct {
	hist  *hdrhistogram.Histogram
	count int64
	errs  int64
}

// histogramRange spans 1µs..60s with 3 sig figs. Beyond 60s most scenarios
// have already timed out at the HTTP client; if a future scenario needs a
// wider range, bump only minLatency / maxLatency, not sigFigs (which is the
// memory knob).
const (
	minLatency = 1                // microseconds — HDR works in int64 ticks
	maxLatency = 60 * 1_000 * 1_000 // 60s in microseconds
	sigFigs    = 3
)

// NewRecorder allocates a fresh recorder. The startedAt timestamp anchors
// the timeline buckets — every observation's elapsed offset is computed
// against this value.
func NewRecorder(startedAt time.Time) *Recorder {
	return &Recorder{
		startedAt:   startedAt,
		global:      hdrhistogram.New(minLatency, maxLatency, sigFigs),
		perEndpoint: map[string]*endpointStats{},
	}
}

// Record observes a single Result. Histogram values are stored in
// microseconds; latencies above maxLatency are clamped to maxLatency so a
// single rogue 90-second request does not corrupt the histogram.
func (r *Recorder) Record(res Result, observedAt time.Time) {
	r.totalReqs.Add(1)
	r.totalBody.Add(res.Bytes)
	if res.Err != nil {
		r.totalErrs.Add(1)
	}
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		r.total2xx.Add(1)
	case res.StatusCode >= 400 && res.StatusCode < 500:
		r.total4xx.Add(1)
	case res.StatusCode >= 500:
		r.total5xx.Add(1)
	}

	micros := res.Latency.Microseconds()
	if micros < minLatency {
		micros = minLatency
	}
	if micros > maxLatency {
		micros = maxLatency
	}

	endpoint := res.Endpoint
	if endpoint == "" {
		endpoint = "(unnamed)"
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	_ = r.global.RecordValue(micros)

	stats := r.perEndpoint[endpoint]
	if stats == nil {
		stats = &endpointStats{
			hist: hdrhistogram.New(minLatency, maxLatency, sigFigs),
		}
		r.perEndpoint[endpoint] = stats
	}
	_ = stats.hist.RecordValue(micros)
	stats.count++
	stats.bytes += res.Bytes
	if res.Err != nil {
		stats.errs++
	}
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		stats.c2xx++
	case res.StatusCode >= 400 && res.StatusCode < 500:
		stats.c4xx++
	case res.StatusCode >= 500:
		stats.c5xx++
	}

	bucketIdx := int(observedAt.Sub(r.startedAt).Seconds())
	if bucketIdx < 0 {
		bucketIdx = 0
	}
	for len(r.timeline) <= bucketIdx {
		r.timeline = append(r.timeline, &timelineBucket{
			hist: hdrhistogram.New(minLatency, maxLatency, sigFigs),
		})
	}
	bucket := r.timeline[bucketIdx]
	_ = bucket.hist.RecordValue(micros)
	bucket.count++
	if res.Err != nil {
		bucket.errs++
	}
}

// LiveCounters returns the current totals without locking the histogram.
// Used by the runner's progress log line every few seconds.
func (r *Recorder) LiveCounters() (total, errs, ok int64) {
	return r.totalReqs.Load(), r.totalErrs.Load(), r.total2xx.Load()
}

// Snapshot is a read-only summary suitable for rendering. It copies values
// out of the histograms so the report renderer does not have to hold the
// mutex during template execution.
type Snapshot struct {
	StartedAt    time.Time
	EndedAt      time.Time
	Duration     time.Duration
	TotalReqs    int64
	TotalErrs    int64
	Total2xx     int64
	Total4xx     int64
	Total5xx     int64
	TotalBody    int64
	Global       LatencyStats
	PerEndpoint  []EndpointSnapshot
	Timeline     []TimelinePoint
	HistogramCDF []HistogramPoint // global CDF for the chart in the report
}

// LatencyStats holds the percentile bundle in milliseconds. Microseconds
// are the storage unit; the renderer wants milliseconds.
type LatencyStats struct {
	Count int64
	Min   float64
	Mean  float64
	Max   float64
	P50   float64
	P90   float64
	P95   float64
	P99   float64
	P999  float64
}

type EndpointSnapshot struct {
	Endpoint   string
	Count      int64
	Errs       int64
	C2xx       int64
	C4xx       int64
	C5xx       int64
	BytesTotal int64
	Latency    LatencyStats
}

type TimelinePoint struct {
	SecondOffset int     // 0-based seconds since startedAt
	RPS          float64 // requests in this 1s bucket
	Errors       int64
	P50ms        float64
	P95ms        float64
	P99ms        float64
}

// HistogramPoint is one (latency, cumulative count) pair for the report
// histogram. Buckets are picked at exponentially increasing latencies so the
// chart stays compact across 5 orders of magnitude.
type HistogramPoint struct {
	LatencyMs float64
	Cumul     int64
}

// Snapshot copies the recorder state for the report. Holds the lock for the
// duration of the copy (typically <1ms even with millions of samples — HDR
// percentile lookup is constant-time).
func (r *Recorder) Snapshot(endedAt time.Time) Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()

	snap := Snapshot{
		StartedAt: r.startedAt,
		EndedAt:   endedAt,
		Duration:  endedAt.Sub(r.startedAt),
		TotalReqs: r.totalReqs.Load(),
		TotalErrs: r.totalErrs.Load(),
		Total2xx:  r.total2xx.Load(),
		Total4xx:  r.total4xx.Load(),
		Total5xx:  r.total5xx.Load(),
		TotalBody: r.totalBody.Load(),
		Global:    statsFromHist(r.global, r.totalReqs.Load()),
	}

	keys := make([]string, 0, len(r.perEndpoint))
	for k := range r.perEndpoint {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	snap.PerEndpoint = make([]EndpointSnapshot, 0, len(keys))
	for _, k := range keys {
		s := r.perEndpoint[k]
		snap.PerEndpoint = append(snap.PerEndpoint, EndpointSnapshot{
			Endpoint:   k,
			Count:      s.count,
			Errs:       s.errs,
			C2xx:       s.c2xx,
			C4xx:       s.c4xx,
			C5xx:       s.c5xx,
			BytesTotal: s.bytes,
			Latency:    statsFromHist(s.hist, s.count),
		})
	}

	snap.Timeline = make([]TimelinePoint, 0, len(r.timeline))
	for i, b := range r.timeline {
		if b == nil {
			continue
		}
		snap.Timeline = append(snap.Timeline, TimelinePoint{
			SecondOffset: i,
			RPS:          float64(b.count),
			Errors:       b.errs,
			P50ms:        usToMs(b.hist.ValueAtQuantile(50)),
			P95ms:        usToMs(b.hist.ValueAtQuantile(95)),
			P99ms:        usToMs(b.hist.ValueAtQuantile(99)),
		})
	}

	snap.HistogramCDF = histogramCDF(r.global)
	return snap
}

func statsFromHist(h *hdrhistogram.Histogram, count int64) LatencyStats {
	if count == 0 {
		return LatencyStats{}
	}
	return LatencyStats{
		Count: count,
		Min:   usToMs(h.Min()),
		Mean:  float64(h.Mean()) / 1000.0,
		Max:   usToMs(h.Max()),
		P50:   usToMs(h.ValueAtQuantile(50)),
		P90:   usToMs(h.ValueAtQuantile(90)),
		P95:   usToMs(h.ValueAtQuantile(95)),
		P99:   usToMs(h.ValueAtQuantile(99)),
		P999:  usToMs(h.ValueAtQuantile(99.9)),
	}
}

func usToMs(v int64) float64 {
	return float64(v) / 1000.0
}

// histogramCDF samples the histogram at log-spaced latencies between min and
// max so the report chart shows the body of the distribution and the long
// tail in a single view. ~40 points is enough for a smooth curve without
// bloating the embedded JSON.
func histogramCDF(h *hdrhistogram.Histogram) []HistogramPoint {
	if h.TotalCount() == 0 {
		return nil
	}
	const points = 40
	min := float64(h.Min())
	max := float64(h.Max())
	if max <= min {
		return []HistogramPoint{{LatencyMs: usToMs(int64(min)), Cumul: h.TotalCount()}}
	}
	out := make([]HistogramPoint, 0, points)
	for i := 0; i <= points; i++ {
		// Geometric interpolation in microseconds.
		t := float64(i) / float64(points)
		latency := min * pow(max/min, t)
		v := int64(latency)
		// HDR doesn't expose CDF directly — derive from cumulative count
		// up to value via Distribution iteration. For a small fixed point
		// count we just walk the bars.
		var cumul int64
		for _, b := range h.Distribution() {
			if b.From > v {
				break
			}
			cumul += b.Count
		}
		out = append(out, HistogramPoint{LatencyMs: usToMs(v), Cumul: cumul})
	}
	return out
}

// pow is a local helper kept so future replacements (precomputed log/exp,
// table lookup) are a one-line change. The CDF runs once at the end so this
// is not on the hot path.
func pow(base, exp float64) float64 {
	return math.Pow(base, exp)
}
