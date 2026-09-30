package analysis

import (
	"fmt"
	"math"

	"github.com/wasmbench/wasmbench/experiment"
)

const PilotVersion = experiment.PilotVersion

type PilotCell struct {
	Runtime           string   `json:"runtime"`
	Workload          string   `json:"workload"`
	Scenario          string   `json:"scenario"`
	Status            string   `json:"status"`
	Reason            string   `json:"reason,omitempty"`
	Summary           *Summary `json:"pilot_summary,omitempty"`
	RelativeHalfWidth *float64 `json:"relative_ci_half_width"`
	Launches          int      `json:"chosen_launches"`
}
type PilotPlan struct {
	Version                 string      `json:"version"`
	AnalysisVersion         string      `json:"analysis_version"`
	SourceBundle            string      `json:"source_bundle"`
	SourceChecksumsSHA256   string      `json:"source_checksums_sha256"`
	SourceLockSHA256        string      `json:"source_lock_sha256"`
	TargetRelativeHalfWidth float64     `json:"target_relative_ci_half_width"`
	MinLaunches             int         `json:"min_launches"`
	MaxLaunches             int         `json:"max_launches"`
	Launches                int         `json:"chosen_launches"`
	Status                  string      `json:"status"`
	Method                  string      `json:"method"`
	Cells                   []PilotCell `json:"cells"`
}

// PlanPilot fixes a future budget; it never changes or pools the pilot data.
// Square-root scaling is an explicitly labeled planning heuristic, not a
// precision guarantee or a statistical power calculation for runtime ratios.
func PlanPilot(b experiment.Bundle, target float64, minLaunches, maxLaunches int) (PilotPlan, error) {
	if !experiment.HostBaselineAllowsMeasurements(b.Manifest) {
		return PilotPlan{}, fmt.Errorf("pilot host baseline did not match")
	}
	if len(b.Manifest.Lock.PilotPlan) > 0 {
		return PilotPlan{}, fmt.Errorf("confirmation data cannot be recycled as a pilot; use a separately declared pilot experiment")
	}
	p := PilotPlan{Version: PilotVersion, AnalysisVersion: Version, SourceLockSHA256: b.Manifest.LockSHA256, TargetRelativeHalfWidth: target, MinLaunches: minLaunches, MaxLaunches: maxLaunches, Status: "ready", Method: "ceil(n * (observed bootstrap relative half-width / target)^2), bounded below by minimum; common maximum across all cells; heuristic only, not a precision or power guarantee; fresh confirmation data required"}
	if b.Manifest.Kind != "measurement" || b.Manifest.Lock.Options.Check || b.Manifest.Lock.Options.Profile != "timing" {
		return p, fmt.Errorf("pilot requires a timing measurement bundle, not correctness or instrumented data")
	}
	if math.IsNaN(target) || math.IsInf(target, 0) || target <= 0 || target >= 1 || minLaunches < 6 || maxLaunches < minLaunches || maxLaunches > 10000 {
		return p, fmt.Errorf("target must be in (0,1); 6 <= min-launches <= max-launches <= 10000")
	}
	lock := b.Manifest.Lock
	if len(lock.Workloads) == 0 || len(lock.Runtimes) == 0 || len(lock.Options.Scenarios) == 0 {
		return p, fmt.Errorf("empty pilot matrix")
	}
	summaries := map[string]Summary{}
	key := func(r, w, s string) string { return r + "\x00" + w + "\x00" + s }
	invalid := map[string]string{}
	blocks := map[string]map[int]bool{}
	for _, t := range b.Trials {
		k := key(t.Runtime, t.Workload, t.Scenario)
		if t.Block < 0 {
			if t.Status != "ok" {
				invalid[k] = "pilot admission did not succeed"
			}
			continue
		}
		if blocks[k] == nil {
			blocks[k] = map[int]bool{}
		}
		if t.Profile != "timing" || t.Block >= lock.Options.Launches || blocks[k][t.Block] {
			invalid[k] = "duplicate/out-of-range block or inconsistent profile"
		}
		blocks[k][t.Block] = true
		for _, s := range t.Samples {
			if !s.Verified || s.ElapsedNS < 0 || s.Operations <= 0 {
				invalid[k] = "invalid or unverified samples cannot be filtered out of a pilot"
			}
		}
	}
	for _, s := range Summarize(b) {
		summaries[key(s.Runtime, s.Workload, s.Scenario)] = s
	}
	for _, r := range lock.Runtimes {
		for _, w := range lock.Workloads {
			for _, scenario := range lock.Options.Scenarios {
				c := PilotCell{Runtime: r.ID, Workload: w.ID, Scenario: scenario, Status: "unresolved"}
				s, ok := summaries[key(r.ID, w.ID, scenario)]
				if ok {
					copy := s
					c.Summary = &copy
				}
				switch {
				case invalid[key(r.ID, w.ID, scenario)] != "":
					c.Reason = invalid[key(r.ID, w.ID, scenario)]
				case !ok:
					c.Reason = "missing experiment cell"
				case s.Attempted != lock.Options.Launches || s.Launches != lock.Options.Launches || s.Failures["ok"] != lock.Options.Launches:
					c.Reason = "pilot requires every declared launch to succeed with verified timing samples; failures and unsupported cells cannot be dropped"
				case s.Launches < 6:
					c.Reason = "at least six independent pilot launches required"
				case s.Median == nil || *s.Median <= 0 || s.CILow == nil || s.CIHigh == nil:
					c.Reason = "positive median and bootstrap interval required"
				case s.Stability == "unstable_within_launch":
					c.Reason = "within-launch drift detected; increasing process count does not establish steady state"
				default:
					half := math.Max(*s.Median-*s.CILow, *s.CIHigh-*s.Median) / *s.Median
					c.RelativeHalfWidth = &half
					estimate := math.Ceil(float64(s.Launches) * math.Pow(half/target, 2))
					if math.IsNaN(estimate) || math.IsInf(estimate, 0) || estimate > float64(maxLaunches) {
						c.Status = "budget_exceeded"
						c.Reason = "estimated budget exceeds cap; no clipped precision claim"
					} else {
						c.Launches = max(minLaunches, int(estimate))
						c.Status = "ready"
						p.Launches = max(p.Launches, c.Launches)
					}
				}
				if c.Status != "ready" {
					p.Status = "unresolved"
				}
				p.Cells = append(p.Cells, c)
			}
		}
	}
	if p.Status != "ready" {
		p.Launches = 0
	}
	return p, nil
}
