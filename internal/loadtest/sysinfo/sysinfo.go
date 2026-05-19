// Package sysinfo captures the host environment a load test ran against,
// so a report consumer can answer "is this 8000 RPS number meaningful?"
// without remembering whether the run was on a Mac laptop on battery or a
// dedicated workstation.
package sysinfo

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

// Snapshot is the small bundle embedded in every report.
type Snapshot struct {
	Hostname    string    `json:"hostname"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	NumCPU      int       `json:"num_cpu"`
	GoVersion   string    `json:"go_version"`
	GeneratedAt time.Time `json:"generated_at"`
	BackendURL  string    `json:"backend_url"`
	BackendBuild string   `json:"backend_build,omitempty"` // populated from /healthz if available
}

// Capture fills in the cheap fields. We deliberately do not call out to
// gopsutil here — the hardware fingerprint we want (CPU count, OS, arch) is
// already in stdlib, and pulling in /proc lookups would add startup latency
// to every load-test run.
func Capture(ctx context.Context, backendURL string) Snapshot {
	host, _ := hostname()
	return Snapshot{
		Hostname:    host,
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		NumCPU:      runtime.NumCPU(),
		GoVersion:   runtime.Version(),
		GeneratedAt: time.Now(),
		BackendURL:  backendURL,
	}
}

func hostname() (string, error) {
	h, err := osHostname()
	if err != nil {
		return "unknown", err
	}
	if h == "" {
		return "unknown", fmt.Errorf("empty hostname")
	}
	return h, nil
}
