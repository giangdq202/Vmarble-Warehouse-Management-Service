package loadtest

import (
	"context"
	"net/http"
)

// Scenario is one named load-test workload. The runner calls Step in a loop
// (closed-loop) or on a rate-limited cadence (open-loop), feeding the
// returned Result into the recorder.
//
// Implementations should be cheap to construct and safe for concurrent use:
// the runner spins up N virtual users, each calling Step against the same
// Scenario value. Per-request state (URL, body) lives inside Step; shared
// state (HTTP client, base URL, auth header) lives on the struct.
//
// A Scenario is registered in the scenarios package via Register; the CLI
// resolves -scenario=<name> against that registry.
type Scenario interface {
	Name() string
	// Step performs one logical user action and returns the observation.
	// vu is the 0-based virtual user index, useful for scenarios that need
	// per-VU state (e.g. unique idempotency keys).
	Step(ctx context.Context, vu int) Result
}

// Client bundles the dialer + auth header used by every Scenario. Sharing a
// single *http.Client across VUs is intentional — Go's transport keeps a
// connection pool keyed by host, and starving it per-VU would dominate
// latency at high VU counts.
type Client struct {
	HTTP    *http.Client
	BaseURL string // e.g. "http://localhost:8080"
	Token   string // optional Bearer token; empty disables the header
}

// Do executes req with the shared client and Bearer header (if any).
// Helper kept on the type so scenarios don't reimplement header handling.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("Accept", "application/json")
	return c.HTTP.Do(req)
}
