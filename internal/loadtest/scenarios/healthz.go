package scenarios

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
)

// Healthz hits /healthz repeatedly. It is the cheapest possible load — the
// handler does a 2-second timeout DB ping and returns a small JSON object —
// so it isolates HTTP-layer overhead from query cost.
//
// Use this as the baseline before pointing harder scenarios at the same box:
// if Healthz P99 already shows queueing, the cap is at the network or the
// connection pool, not in your business logic.
type Healthz struct {
	Client *loadtest.Client
}

func (s *Healthz) Name() string { return "healthz" }

func (s *Healthz) Step(ctx context.Context, _ int) loadtest.Result {
	const endpoint = "GET /healthz"
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.Client.BaseURL+"/healthz", nil)
	if err != nil {
		return loadtest.Result{Endpoint: endpoint, Err: err, Latency: time.Since(start)}
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return loadtest.Result{Endpoint: endpoint, Err: err, Latency: time.Since(start)}
	}
	defer func() { _ = resp.Body.Close() }()
	n, _ := io.Copy(io.Discard, resp.Body)
	return loadtest.Result{
		Endpoint:   endpoint,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Bytes:      n,
	}
}
