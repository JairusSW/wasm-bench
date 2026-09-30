package publish

import (
	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"sort"
)

type ScalingTrialLink struct {
	ID       string `json:"id"`
	Runtime  string `json:"runtime"`
	Workload string `json:"workload"`
	Scenario string `json:"scenario"`
	Status   string `json:"status"`
}

// Restrict each configuration's curves to exact matched workload contracts.
// Keep raw failure links even when no numeric observation was available.
func pairedMemoryScaling(memory experiment.Bundle, matched map[string]bool) ([]analysis.ScalingCurve, []ScalingTrialLink) {
	ids := map[string]bool{}
	var links []ScalingTrialLink
	for _, t := range memory.Trials {
		if t.Block >= 0 && matched[t.Runtime+"\x00"+t.Workload] {
			ids[t.Runtime] = true
			links = append(links, ScalingTrialLink{ID: t.ID, Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Status: t.Status})
		}
	}
	var runtimes []string
	for id := range ids {
		runtimes = append(runtimes, id)
	}
	sort.Strings(runtimes)
	var curves []analysis.ScalingCurve
	for _, id := range runtimes {
		b := memory
		b.Manifest.Lock.Workloads = nil
		b.Trials = nil
		for _, w := range memory.Manifest.Lock.Workloads {
			if matched[id+"\x00"+w.ID] {
				b.Manifest.Lock.Workloads = append(b.Manifest.Lock.Workloads, w)
			}
		}
		for _, t := range memory.Trials {
			if t.Runtime == id && matched[id+"\x00"+t.Workload] {
				b.Trials = append(b.Trials, t)
			}
		}
		curves = append(curves, analysis.Scaling(b)...)
	}
	return curves, links
}
