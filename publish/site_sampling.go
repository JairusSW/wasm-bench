package publish

import (
	"fmt"
	"sort"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

type SiteSamplingGroup struct {
	Schema         int       `json:"schema"`
	ID             string    `json:"id"`
	PassID         string    `json:"passId"`
	CapturedAt     time.Time `json:"capturedAt"`
	Runtime        string    `json:"runtime"`
	Workload       string    `json:"workload"`
	Scenario       string    `json:"scenario"`
	Profile        string    `json:"profile"`
	ManifestSHA256 string    `json:"manifestSha256"`
	TrialsSHA256   string    `json:"trialsSha256"`
	TrialCount     int       `json:"trialCount"`
}

// Source identities survive report rebuilds but never equate matching block
// numbers from different passes. A missing source pass yields no inferred group.
func siteSamplingGroup(bundles []experiment.Bundle, pass, runtime, workload, scenario, profile string, selected map[string]bool) (*SiteSamplingGroup, error) {
	var source *experiment.Bundle
	for i := range bundles {
		if bundles[i].Manifest.ID == pass {
			if source != nil {
				return nil, fmt.Errorf("duplicate sampling source pass")
			}
			source = &bundles[i]
		}
	}
	if source == nil {
		return nil, nil
	}
	if source.Manifest.Created.IsZero() {
		return nil, nil
	}
	if source.Manifest.Lock.Options.Profile != profile {
		return nil, fmt.Errorf("sampling profile differs from source pass")
	}
	trials := [][2]string{}
	seen := map[string]bool{}
	for _, trial := range source.Trials {
		if trial.Runtime != runtime || trial.Workload != workload || trial.Scenario != scenario || trial.Profile != profile || selected != nil && !selected[trial.ID] {
			continue
		}
		if seen[trial.ID] {
			return nil, fmt.Errorf("duplicate sampling trial identity")
		}
		seen[trial.ID] = true
		digest, err := siteID(trial)
		if err != nil {
			return nil, err
		}
		trials = append(trials, [2]string{trial.ID, digest})
	}
	if selected != nil {
		for id, include := range selected {
			if include && !seen[id] {
				return nil, fmt.Errorf("sampling references absent source trial")
			}
		}
	}
	if len(trials) == 0 {
		return nil, nil
	}
	sort.Slice(trials, func(i, j int) bool { return trials[i][0] < trials[j][0] })
	manifest, err := siteID(source.Manifest)
	if err != nil {
		return nil, err
	}
	digest, err := siteID(trials)
	if err != nil {
		return nil, err
	}
	group := SiteSamplingGroup{Schema: 1, PassID: pass, CapturedAt: source.Manifest.Created, Runtime: runtime, Workload: workload, Scenario: scenario, Profile: profile, ManifestSHA256: manifest, TrialsSHA256: digest, TrialCount: len(trials)}
	group.ID, err = siteID(group)
	if err != nil {
		return nil, err
	}
	return &group, nil
}
