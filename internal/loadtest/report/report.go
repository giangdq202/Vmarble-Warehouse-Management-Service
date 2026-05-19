// Package report turns a loadtest.Snapshot into a self-contained HTML page
// plus a sibling summary.json. The HTML embeds Chart.js from CDN so the
// file works offline only after first load — acceptable because operators
// open it on the same workstation that ran the test.
//
// Why HTML instead of vegeta-style ASCII or a Grafana dashboard?
//   - ASCII percentile bundles bury the time-series view ("did P95 climb at
//     minute 3?"), which is the most common diagnostic question.
//   - Grafana would require a metrics pipeline and a dashboard JSON; for a
//     local stretch test that's strict overkill.
//   - HTML + Chart.js + an embedded summary keeps the artifact movable:
//     drop the file in a Slack thread or a GitHub issue and the recipient
//     can interrogate the run without rerunning it.
package report

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/vmarble/warehouse-management-service/internal/loadtest"
	"github.com/vmarble/warehouse-management-service/internal/loadtest/sysinfo"
)

//go:embed template.html
var tpl string

// Bundle is what gets serialized into the HTML page (as a <script> JSON
// blob) and into summary.json next to it. Keeping a single struct means the
// JSON file and the page can never drift.
type Bundle struct {
	GeneratedAt time.Time          `json:"generated_at"`
	Scenario    string             `json:"scenario"`
	Mode        string             `json:"mode"`
	VUs         int                `json:"vus"`
	RPSTarget   float64            `json:"rps_target"`
	DurationS   float64            `json:"duration_s"`
	Host        sysinfo.Snapshot   `json:"host"`
	Snapshot    loadtest.Snapshot  `json:"snapshot"`
	Thresholds  []ThresholdResult  `json:"thresholds,omitempty"`
}

// ThresholdResult is a pre-evaluated SLO check shown at the top of the
// report — operators want a single green/red answer, not a percentile table
// they have to interpret.
type ThresholdResult struct {
	Name   string  `json:"name"`
	Want   string  `json:"want"`
	Got    string  `json:"got"`
	Passed bool    `json:"passed"`
	Detail string  `json:"detail,omitempty"`
}

// Write renders the report into outDir/<run-id>/{report.html, summary.json}
// and returns the directory path. The run-id is derived from the snapshot
// start time so reruns don't clobber each other.
func Write(outDir string, b Bundle) (string, error) {
	runID := b.Snapshot.StartedAt.Format("20060102-150405") + "-" + safeName(b.Scenario)
	dir := filepath.Join(outDir, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("loadtest report: mkdir: %w", err)
	}

	if err := writeJSON(filepath.Join(dir, "summary.json"), b); err != nil {
		return "", err
	}

	t, err := template.New("report").Funcs(template.FuncMap{
		"json": func(v any) (template.JS, error) {
			raw, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			return template.JS(raw), nil
		},
		"ms": func(v float64) string { return fmt.Sprintf("%.2f ms", v) },
		"int": func(v int64) string { return fmt.Sprintf("%d", v) },
	}).Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("loadtest report: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, b); err != nil {
		return "", fmt.Errorf("loadtest report: execute template: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.html"), buf.Bytes(), 0o644); err != nil {
		return "", fmt.Errorf("loadtest report: write html: %w", err)
	}
	return dir, nil
}

func writeJSON(path string, b Bundle) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("loadtest report: create json: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(b); err != nil {
		return fmt.Errorf("loadtest report: encode json: %w", err)
	}
	return nil
}

// WriteJSONOnly is exposed so tests can validate the JSON contract without
// pulling in the HTML template surface.
func WriteJSONOnly(w io.Writer, b Bundle) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(b)
}

func safeName(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		case c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "run"
	}
	return string(out)
}
