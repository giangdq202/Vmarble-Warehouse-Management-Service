package scenarios

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
)

// ListPaged is the building block behind every read-mostly scenario in the
// load-test suite. It paginates over a list endpoint and emits one Result
// per request — the runner does not care that it took 5 HTTP calls to walk
// the dataset, only that each call was observed.
//
// Why a generic paginator instead of one scenario per route?
//   - The list endpoints all share the same envelope (httpkit.PagedResult)
//     and the same query knobs (page, limit, search, sort_by, order).
//   - The interesting variable for load testing is the route, not the
//     pagination mechanics. Folding mechanics here keeps each scenario file
//     to its endpoint name and a couple of params.
type ListPaged struct {
	ScenarioName string
	Path         string // e.g. "/api/v1/work-orders"
	Limit        int    // page size; 100 is the server cap
	Client       *loadtest.Client
	// page is incremented per request so VUs sweep through the dataset
	// instead of all hammering page=1. Cheap synthetic load that stays
	// honest about pagination cost.
	page atomic.Int64
}

func (s *ListPaged) Name() string { return s.ScenarioName }

func (s *ListPaged) Step(ctx context.Context, _ int) loadtest.Result {
	page := s.page.Add(1)
	q := url.Values{}
	q.Set("page", strconv.FormatInt(page, 10))
	if s.Limit > 0 {
		q.Set("limit", strconv.Itoa(s.Limit))
	}
	endpoint := fmt.Sprintf("GET %s", s.Path)
	full := s.Client.BaseURL + s.Path + "?" + q.Encode()

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return loadtest.Result{Endpoint: endpoint, Err: err, Latency: time.Since(start)}
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return loadtest.Result{Endpoint: endpoint, Err: err, Latency: time.Since(start)}
	}
	defer func() { _ = resp.Body.Close() }()

	// Drain body so the connection returns to the pool. We don't parse the
	// payload — load tests should reflect server work, not client-side
	// JSON decode cost.
	n, _ := io.Copy(io.Discard, resp.Body)
	return loadtest.Result{
		Endpoint:   endpoint,
		StatusCode: resp.StatusCode,
		Latency:    time.Since(start),
		Bytes:      n,
	}
}
