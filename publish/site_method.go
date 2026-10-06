package publish

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// A source-owned, bounded selector. Numerical analysis stays in the verified
// report builder. A missing collector is explicit, never inferred from an OS.
type SiteMethod struct {
	Schema          int                       `json:"schema"`
	Status          string                    `json:"status"`
	Reason          string                    `json:"reason,omitempty"`
	Metric          string                    `json:"metric"`
	Scenario        string                    `json:"scenario"`
	Profile         string                    `json:"profile"`
	Statistic       string                    `json:"statistic"`
	Recipe          json.RawMessage           `json:"recipe,omitempty"`
	RecipeSHA256    string                    `json:"recipeSha256,omitempty"`
	CollectorStatus string                    `json:"collectorStatus"`
	Observations    []SiteObservationIdentity `json:"observations"`
}
type SiteObservationIdentity struct {
	DefinitionVersion int    `json:"definitionVersion"`
	Unit              string `json:"unit"`
	Scope             string `json:"scope"`
	Phase             string `json:"phase"`
	Collector         string `json:"collector"`
	CollectorVersion  string `json:"collectorVersion"`
	Quality           string `json:"quality"`
	Profile           string `json:"profile"`
	Denominator       string `json:"denominator"`
}

func siteMethod(bundles []experiment.Bundle, pass, runtime, workload, scenario, profile, metric, statistic string, trials map[string]bool) (SiteMethod, error) {
	m := SiteMethod{Schema: 1, Status: "unavailable", Reason: "source pass context not exported", Metric: metric, Scenario: scenario, Profile: profile, Statistic: statistic, CollectorStatus: "not_recorded", Observations: []SiteObservationIdentity{}}
	var source *experiment.Bundle
	for i := range bundles {
		if bundles[i].Manifest.ID == pass {
			if source != nil {
				return m, fmt.Errorf("duplicate source pass identity")
			}
			source = &bundles[i]
		}
	}
	if source == nil {
		return m, nil
	}
	if source.Manifest.Lock.Options.Profile != profile {
		return m, fmt.Errorf("result profile differs from source pass")
	}
	options := source.Manifest.Lock.Options
	// Suite and other scenarios describe scheduling, not this measurement recipe.
	options.Suite = ""
	options.Scenarios = []string{scenario}
	if n, ok := options.ScenarioSamples[scenario]; ok {
		options.Samples = n
	}
	options.ScenarioSamples = nil
	optionBytes, _ := siteJSON(options)
	var optionValues map[string]json.RawMessage
	if e := json.Unmarshal(optionBytes, &optionValues); e != nil {
		return m, e
	}
	for key, value := range map[string]int64{"seed": options.Seed, "timeout_ns": int64(options.Timeout), "sustained_duration_ns": int64(options.SustainedDuration)} {
		if value > (1<<53-1) || value < -(1<<53-1) {
			optionValues[key], _ = siteJSON(strconv.FormatInt(value, 10))
		}
	}
	recipe := map[string]any{"protocol": source.Manifest.Lock.Protocol, "runnerVersion": source.Manifest.Lock.RunnerVersion, "runnerSha256": source.Manifest.Lock.RunnerSHA256, "options": optionValues, "hostPolicy": source.Manifest.Lock.HostPolicy, "requireIRQAffinity": source.Manifest.Lock.RequireIRQAffinity, "requireIsolatedCPUPartition": source.Manifest.Lock.RequireIsolatedCPUPartition}
	b, e := siteJSON(recipe)
	if e != nil {
		return m, e
	}
	if len(b) > 32*1024 {
		return m, fmt.Errorf("measurement recipe exceeds ceiling")
	}
	m.Recipe = b
	m.RecipeSHA256, _ = siteID(recipe)
	m.Status = "available"
	m.Reason = ""
	identities := map[string]SiteObservationIdentity{}
	add := func(o protocol.Observation, operations int) {
		if _, ok := memoryObservationValue(o, metric, scenario, operations); !ok {
			return
		}
		v := SiteObservationIdentity{o.DefinitionVersion, o.Unit, o.Scope, o.Phase, o.Collector, o.CollectorVersion, o.Quality, o.Profile, o.Denominator}
		key, _ := siteID(v)
		identities[key] = v
	}
	for _, t := range source.Trials {
		if t.Runtime != runtime || t.Workload != workload || t.Scenario != scenario || t.Profile != profile || t.Status != "ok" || (trials != nil && !trials[t.ID]) {
			continue
		}
		if metric == "process.rss" || metric == "process.peak_rss" {
			for _, o := range t.Observations {
				add(o, 1)
			}
		} else if metric != "time.wall" && metric != "native.code_size" {
			for _, s := range t.Samples {
				if s.Verified && !s.Warmup {
					for _, o := range s.Observations {
						add(o, s.Operations)
					}
				}
			}
		}
	}
	if len(identities) > 32 {
		return m, fmt.Errorf("measurement collector identities exceed ceiling")
	}
	keys := []string{}
	for k := range identities {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		o := identities[k]
		for _, value := range []string{o.Unit, o.Scope, o.Phase, o.Collector, o.CollectorVersion, o.Quality, o.Profile, o.Denominator} {
			if len(value) > 4096 {
				return m, fmt.Errorf("observation identity exceeds ceiling")
			}
		}
		m.Observations = append(m.Observations, o)
	}
	if len(keys) > 0 {
		m.CollectorStatus = "recorded"
	}
	encoded, e := siteJSON(m)
	if e != nil || len(encoded) > 64*1024 {
		return m, fmt.Errorf("measurement method exceeds ceiling")
	}
	return m, nil
}
