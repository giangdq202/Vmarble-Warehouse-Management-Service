package loadtest

import (
	"errors"
	"testing"
	"time"
)

func TestRecorder_RecordsBasicCounts(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	r := NewRecorder(start)

	r.Record(Result{Endpoint: "GET /a", StatusCode: 200, Latency: 10 * time.Millisecond, Bytes: 100}, start)
	r.Record(Result{Endpoint: "GET /a", StatusCode: 200, Latency: 20 * time.Millisecond, Bytes: 100}, start.Add(time.Second))
	r.Record(Result{Endpoint: "GET /b", StatusCode: 500, Latency: 5 * time.Millisecond}, start.Add(time.Second))
	r.Record(Result{Endpoint: "GET /b", StatusCode: 0, Latency: 30 * time.Second, Err: errors.New("boom")}, start.Add(2*time.Second))

	total, errs, ok := r.LiveCounters()
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if errs != 1 {
		t.Errorf("errs = %d, want 1", errs)
	}
	if ok != 2 {
		t.Errorf("2xx = %d, want 2", ok)
	}

	snap := r.Snapshot(start.Add(3 * time.Second))
	if got := snap.Total5xx; got != 1 {
		t.Errorf("5xx = %d, want 1", got)
	}
	if got := snap.TotalBody; got != 200 {
		t.Errorf("body bytes = %d, want 200", got)
	}
	if len(snap.PerEndpoint) != 2 {
		t.Fatalf("per-endpoint count = %d, want 2", len(snap.PerEndpoint))
	}
	if len(snap.Timeline) != 3 {
		t.Errorf("timeline buckets = %d, want 3 (one per second)", len(snap.Timeline))
	}
}

func TestRecorder_PercentilesMonotonic(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	r := NewRecorder(start)
	for i := 1; i <= 1000; i++ {
		r.Record(Result{Endpoint: "GET /x", StatusCode: 200, Latency: time.Duration(i) * time.Millisecond}, start)
	}
	snap := r.Snapshot(start.Add(time.Second))
	if !(snap.Global.P50 <= snap.Global.P90 && snap.Global.P90 <= snap.Global.P95 && snap.Global.P95 <= snap.Global.P99 && snap.Global.P99 <= snap.Global.P999) {
		t.Errorf("percentiles not monotonic: p50=%.2f p90=%.2f p95=%.2f p99=%.2f p999=%.2f",
			snap.Global.P50, snap.Global.P90, snap.Global.P95, snap.Global.P99, snap.Global.P999)
	}
	// Sanity: with samples 1..1000ms, p50 should be ~500ms ±5% (HDR
	// quantises to 3 sig figs so an exact match is not required).
	if snap.Global.P50 < 470 || snap.Global.P50 > 530 {
		t.Errorf("p50 = %.2f, expected ~500ms", snap.Global.P50)
	}
}

func TestRecorder_ClampsLatencyAtMax(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	r := NewRecorder(start)
	// 90 second outlier — must be clamped to maxLatency (60s) so it
	// doesn't error out the histogram or skew the max past the bound.
	r.Record(Result{Endpoint: "GET /slow", StatusCode: 200, Latency: 90 * time.Second}, start)
	snap := r.Snapshot(start.Add(time.Second))
	// Allow HDR quantisation: max should be at or just below 60s in ms (60_000),
	// definitely not 90_000.
	if snap.Global.Max > 60_500 {
		t.Errorf("max = %.2f, expected clamp near 60_000ms", snap.Global.Max)
	}
}

func TestRecorder_EmptyEndpointFallsBack(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	r := NewRecorder(start)
	r.Record(Result{StatusCode: 200, Latency: 5 * time.Millisecond}, start)
	snap := r.Snapshot(start.Add(time.Second))
	if len(snap.PerEndpoint) != 1 || snap.PerEndpoint[0].Endpoint != "(unnamed)" {
		t.Errorf("expected single (unnamed) endpoint, got %+v", snap.PerEndpoint)
	}
}
