package publish

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
)

const SiteFeaturePolicy = "recorded-feature-trials-v1"

// Counts describe recorded trials, not independent launches, passing assertions,
// or complete specification conformance. Interpretation remains a website policy.
type SiteFeatureScenario struct {
	PassID     string         `json:"passId"`
	Profile    string         `json:"profile"`
	Scenario   string         `json:"scenario"`
	TrialCount int            `json:"trialCount"`
	Outcomes   map[string]int `json:"outcomes"`
}
type SiteFeatureProbe struct {
	Schema          int                   `json:"schema"`
	Policy          string                `json:"policy"`
	ReportID        string                `json:"reportId"`
	EnvironmentID   string                `json:"environmentId"`
	ConfigurationID string                `json:"configurationId"`
	TrackID         string                `json:"trackId"`
	ContractID      string                `json:"contractId"`
	Runtime         string                `json:"runtime"`
	Workload        string                `json:"workload"`
	Feature         string                `json:"feature"`
	Created         time.Time             `json:"created"`
	TrialCount      int                   `json:"trialCount"`
	Scenarios       []SiteFeatureScenario `json:"scenarios"`
	Evidence        []string              `json:"evidence"`
}

func siteFeatureProbes(d Dataset, extra []experiment.Bundle, report, environment string, configurations, tracks, contracts, trials map[string]string, object func(string, any) (string, error), record func(string, string, any) error) error {
	var matchedPasses map[string]map[string]bool
	runtimes := make([]string, 0, len(configurations))
	for runtime := range configurations {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)
	for _, workload := range d.Bundle.Manifest.Lock.Workloads {
		if !strings.HasPrefix(workload.ID, "features/") {
			continue
		}
		var provenance struct {
			Baseline bool `json:"baseline"`
		}
		if len(workload.Provenance) > 0 {
			if err := json.Unmarshal(workload.Provenance, &provenance); err != nil {
				return fmt.Errorf("feature provenance: %w", err)
			}
		}
		if provenance.Baseline {
			continue
		}
		feature := strings.Split(strings.TrimPrefix(workload.ID, "features/"), "/")[0]
		if feature == "" {
			return fmt.Errorf("empty feature probe identity")
		}
		if matchedPasses == nil {
			matchedPasses = map[string]map[string]bool{}
			for _, bundle := range extra {
				matched, err := matchingPassCells(d.Bundle, bundle, bundle.Manifest.Lock.Options.Profile)
				if err != nil {
					return err
				}
				matchedPasses[bundle.Manifest.ID] = matched
			}
		}
		for _, runtime := range runtimes {
			probe := SiteFeatureProbe{Schema: 1, Policy: SiteFeaturePolicy, ReportID: report, EnvironmentID: environment, ConfigurationID: configurations[runtime], TrackID: tracks[runtime], ContractID: contracts[workload.ID], Runtime: runtime, Workload: workload.ID, Feature: feature, Created: d.Bundle.Manifest.Created, Scenarios: []SiteFeatureScenario{}, Evidence: []string{}}
			groups := map[string]*SiteFeatureScenario{}
			for _, bundle := range append([]experiment.Bundle{d.Bundle}, extra...) {
				if bundle.Manifest.ID != d.Bundle.Manifest.ID && !matchedPasses[bundle.Manifest.ID][runtime+"\x00"+workload.ID] {
					continue
				}
				for _, trial := range bundle.Trials {
					if trial.Runtime != runtime || trial.Workload != workload.ID {
						continue
					}
					key := bundle.Manifest.ID + "\x00" + trial.Profile + "\x00" + trial.Scenario
					group := groups[key]
					if group == nil {
						group = &SiteFeatureScenario{PassID: bundle.Manifest.ID, Profile: trial.Profile, Scenario: trial.Scenario, Outcomes: map[string]int{}}
						groups[key] = group
					}
					group.TrialCount++
					group.Outcomes[trial.Status]++
					probe.TrialCount++
					ref := trials[bundle.Manifest.ID+"\x00"+trial.ID]
					if ref == "" {
						return fmt.Errorf("feature trial evidence missing")
					}
					probe.Evidence = append(probe.Evidence, ref)
				}
			}
			keys := make([]string, 0, len(groups))
			for key := range groups {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				probe.Scenarios = append(probe.Scenarios, *groups[key])
			}
			if len(probe.Scenarios) > 64 {
				return fmt.Errorf("feature scenario summary exceeds ceiling")
			}
			var err error
			probe.Evidence, err = siteEvidenceRoots(probe.Evidence, object)
			if err != nil {
				return err
			}
			if len(probe.Evidence) > 16 {
				root, e := object("evidence", map[string]any{"kind": "evidence-index", "schema": 1, "references": probe.Evidence})
				if e != nil {
					return e
				}
				probe.Evidence = []string{root}
			}
			bytes, err := siteJSON(probe)
			if err != nil {
				return err
			}
			if len(bytes)+512 > 10*1024 {
				return fmt.Errorf("feature probe descriptor exceeds ceiling")
			}
			if err = record("feature-probe", siteHash(bytes), probe); err != nil {
				return err
			}
		}
	}
	return nil
}
