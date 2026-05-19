package report

import (
	"fmt"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
)

// Thresholds is a small DSL: pass/fail rules evaluated against a snapshot.
// Each method appends a ThresholdResult; the final slice goes straight into
// the report. Keeping this on the report side (not the runner) means
// thresholds are descriptive, not control flow — the test still finishes
// and writes its data even when SLOs miss, which matters for diagnosis.
type Thresholds struct {
	results []ThresholdResult
}

func NewThresholds() *Thresholds { return &Thresholds{} }

func (t *Thresholds) P95Under(maxMs float64) *Thresholds {
	t.results = append(t.results, ThresholdResult{
		Name: "global.p95",
		Want: fmt.Sprintf("< %.0f ms", maxMs),
	})
	return t
}

func (t *Thresholds) P99Under(maxMs float64) *Thresholds {
	t.results = append(t.results, ThresholdResult{
		Name: "global.p99",
		Want: fmt.Sprintf("< %.0f ms", maxMs),
	})
	return t
}

func (t *Thresholds) ErrorRateUnder(pct float64) *Thresholds {
	t.results = append(t.results, ThresholdResult{
		Name: "error_rate",
		Want: fmt.Sprintf("< %.2f%%", pct),
	})
	return t
}

// Evaluate fills in Got/Passed for every previously-staged threshold,
// using the actual numbers from snap.
func (t *Thresholds) Evaluate(snap loadtest.Snapshot) []ThresholdResult {
	out := make([]ThresholdResult, 0, len(t.results))
	for _, r := range t.results {
		switch r.Name {
		case "global.p95":
			r.Got = fmt.Sprintf("%.2f ms", snap.Global.P95)
			r.Passed = parseLessThan(r.Want, snap.Global.P95)
		case "global.p99":
			r.Got = fmt.Sprintf("%.2f ms", snap.Global.P99)
			r.Passed = parseLessThan(r.Want, snap.Global.P99)
		case "error_rate":
			rate := 0.0
			if snap.TotalReqs > 0 {
				rate = float64(snap.TotalErrs+snap.Total5xx) * 100.0 / float64(snap.TotalReqs)
			}
			r.Got = fmt.Sprintf("%.2f%%", rate)
			r.Passed = parseLessThan(r.Want, rate)
		}
		out = append(out, r)
	}
	return out
}

// parseLessThan reads back the upper bound encoded in the Want string.
// We embed the bound in the human-readable string so the report shows it
// verbatim — and parse it back here rather than carrying a parallel
// numeric field on every result.
func parseLessThan(want string, got float64) bool {
	var bound float64
	// Tolerate both "< 250 ms" and "< 1.50%".
	_, err := fmt.Sscanf(want, "< %f", &bound)
	if err != nil {
		return false
	}
	return got < bound
}
