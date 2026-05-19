package report

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
	"github.com/vmarble/warehouse-management-service/internal/loadtest/sysinfo"
)

func TestWrite_CreatesHTMLAndSummary(t *testing.T) {
	dir := t.TempDir()
	startedAt := time.Date(2026, 5, 19, 10, 30, 0, 0, time.UTC)
	rec := loadtest.NewRecorder(startedAt)
	for i := 1; i <= 100; i++ {
		rec.Record(loadtest.Result{
			Endpoint: "GET /a", StatusCode: 200, Latency: time.Duration(i) * time.Millisecond, Bytes: 50,
		}, startedAt.Add(time.Duration(i)*time.Millisecond))
	}
	snap := rec.Snapshot(startedAt.Add(2 * time.Second))

	bundle := Bundle{
		GeneratedAt: time.Now(),
		Scenario:    "list-pos",
		Mode:        "closed",
		VUs:         8,
		DurationS:   2,
		Host:        sysinfo.Snapshot{Hostname: "test-host", OS: "darwin", Arch: "arm64", NumCPU: 8, BackendURL: "http://localhost:8080"},
		Snapshot:    snap,
		Thresholds: NewThresholds().
			P95Under(500).
			ErrorRateUnder(1).
			Evaluate(snap),
	}
	out, err := Write(dir, bundle)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	for _, name := range []string{"report.html", "summary.json"} {
		path := filepath.Join(out, name)
		st, err := os.Stat(path)
		if err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
		if st.Size() == 0 {
			t.Errorf("%s is empty", path)
		}
	}
}

func TestThresholds_PassFail(t *testing.T) {
	startedAt := time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)
	rec := loadtest.NewRecorder(startedAt)
	for i := 1; i <= 100; i++ {
		rec.Record(loadtest.Result{Endpoint: "GET /a", StatusCode: 200, Latency: time.Duration(i) * time.Millisecond}, startedAt)
	}
	snap := rec.Snapshot(startedAt.Add(time.Second))

	results := NewThresholds().
		P95Under(1000). // pass: max 100ms
		P99Under(50).   // fail: p99 around 99-100ms
		ErrorRateUnder(1).
		Evaluate(snap)

	if !results[0].Passed {
		t.Errorf("P95Under(1000) should pass, got %s", results[0].Got)
	}
	if results[1].Passed {
		t.Errorf("P99Under(50) should fail, got %s", results[1].Got)
	}
	if !results[2].Passed {
		t.Errorf("ErrorRateUnder(1) should pass with no errors, got %s", results[2].Got)
	}
}
