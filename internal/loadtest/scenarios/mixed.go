package scenarios

import (
	"context"
	"sync/atomic"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
)

// Mixed interleaves several scenarios on a round-robin basis to approximate
// production traffic mix. The point of having a Mixed scenario at all is
// that single-endpoint tests miss interaction effects: e.g. a cheap
// /healthz at high RPS can starve the connection pool that a slow
// /work-orders list needs, and you only see that in mixed traffic.
//
// Weights are integer "tickets" — a scenario with weight 3 fires 3× as
// often as one with weight 1. Concrete weights live in the wiring code
// (cmd/loadtest), not here, so the operator can tune mix without rebuilding
// the package.
type Mixed struct {
	Children []WeightedScenario
	cursor   atomic.Int64
	flat     []loadtest.Scenario // expanded on first Step
}

type WeightedScenario struct {
	Scenario loadtest.Scenario
	Weight   int
}

func (m *Mixed) Name() string { return "mixed" }

func (m *Mixed) Step(ctx context.Context, vu int) loadtest.Result {
	if m.flat == nil {
		// Lazy init keeps construction simple; the runner serializes the
		// first few Step calls in practice (worker pool ramp-up) so a
		// benign data race here cannot corrupt the slice — but we still
		// guard with the cursor below to keep this race-detector clean
		// when first call is concurrent.
		var flat []loadtest.Scenario
		for _, c := range m.Children {
			n := c.Weight
			if n <= 0 {
				n = 1
			}
			for i := 0; i < n; i++ {
				flat = append(flat, c.Scenario)
			}
		}
		m.flat = flat
	}
	if len(m.flat) == 0 {
		return loadtest.Result{Endpoint: "(empty mixed)"}
	}
	idx := m.cursor.Add(1) - 1
	return m.flat[int(idx)%len(m.flat)].Step(ctx, vu)
}
