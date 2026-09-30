package analysis

import (
	"fmt"

	"github.com/wasmbench/wasmbench/experiment"
)

type HistoryPoint struct {
	Summaries  []Summary           `json:"timing_summaries,omitempty"`
	Position   int                 `json:"position"`
	Manifest   experiment.Manifest `json:"manifest"`
	Status     string              `json:"status"`
	Reason     string              `json:"reason,omitempty"`
	Comparison *RegressionReport   `json:"comparison,omitempty"`
}

type HistoryReport struct {
	Evidence        []HistoryEvidence `json:"evidence,omitempty"`
	AnalysisVersion string            `json:"analysis_version"`
	Runtime         string            `json:"runtime"`
	BaselineRun     string            `json:"baseline_run"`
	Interpretation  string            `json:"interpretation"`
	Points          []HistoryPoint    `json:"points"`
}

type HistoryEvidence struct {
	Run             string `json:"run"`
	Bundle          string `json:"bundle_locator"`
	ChecksumsSHA256 string `json:"checksums_sha256"`
}

// History consumes already verified bundles in explicit user-supplied order.
// Dates and version strings are provenance, not a trustworthy total ordering.
func History(bundles []experiment.Bundle, runtime string) (HistoryReport, error) {
	r := HistoryReport{AnalysisVersion: "fixed-baseline-history-v1", Runtime: runtime,
		Interpretation: "Explicit input order; first run is the fixed baseline. Each later run is compared independently to that baseline using independent-launch bootstrap intervals, never cross-run block pairing. Incompatible runs and unavailable cells remain visible. No interpolation, change-point attribution, multiple-comparison correction or official regression qualification. Manifests retain exact configuration and provenance; timestamps do not establish revision ancestry.", Points: []HistoryPoint{}}
	if len(bundles) < 2 || runtime == "" {
		return r, fmt.Errorf("history requires at least two runs and a runtime configuration ID")
	}
	seen := map[string]bool{}
	for _, b := range bundles {
		id := b.Manifest.ID
		if id == "" || seen[id] {
			return r, fmt.Errorf("history requires distinct nonempty run identities")
		}
		seen[id] = true
	}
	base := bundles[0]
	if _, err := CompareRuns(base, base, runtime, runtime); err != nil {
		return r, fmt.Errorf("invalid history baseline: %w", err)
	}
	r.BaselineRun = base.Manifest.ID
	for i, b := range bundles {
		p := HistoryPoint{Position: i, Manifest: b.Manifest, Status: "baseline"}
		if b.Manifest.Kind == "measurement" && b.Manifest.Lock.Options.Profile == "timing" {
			for _, summary := range Summarize(b) {
				if summary.Runtime == runtime {
					p.Summaries = append(p.Summaries, summary)
				}
			}
		}
		if i > 0 {
			comparison, err := CompareRuns(base, b, runtime, runtime)
			if err != nil {
				p.Status, p.Reason = "incomparable", err.Error()
			} else {
				p.Status, p.Comparison = "compared", &comparison
			}
		}
		r.Points = append(r.Points, p)
	}
	return r, nil
}
