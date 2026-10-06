package publish

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestSiteMethodSourceCollectorsAndRecipeScope(t *testing.T) {
	b := siteFixture().Bundle
	b.Manifest.ID = "memory-pass"
	b.Manifest.Lock.Options.Profile = "memory"
	b.Manifest.Lock.Options.Scenarios = []string{"steady", "compile"}
	b.Manifest.Lock.Options.ScenarioSamples = map[string]int{"steady": 3, "compile": 90}
	b.Manifest.Lock.Options.Seed = math.MaxInt64
	b.Trials = []experiment.Trial{{ID: "memory-0", Runtime: "engine", Workload: "fixture/a", Scenario: "steady", Profile: "memory", Status: "ok", Observations: []protocol.Observation{{Metric: "process.rss", DefinitionVersion: 1, Value: protocol.Value(0), Unit: "bytes", Scope: "adapter_process", Phase: "steady/after_batch", Collector: "procfs", CollectorVersion: "1", Quality: "boundary_snapshot_only", Profile: "memory", Status: "available", Denominator: "process"}}}}
	method, e := siteMethod([]experiment.Bundle{b}, "memory-pass", "engine", "fixture/a", "steady", "memory", "process.rss", "median_bytes", map[string]bool{"memory-0": true})
	if e != nil || method.Status != "available" || method.CollectorStatus != "recorded" || len(method.Observations) != 1 || method.Observations[0].Denominator != "process" {
		t.Fatal("lost actual source collector", e)
	}
	if !strings.Contains(string(method.Recipe), `"seed":"9223372036854775807"`) {
		t.Fatal("rounded exact seed")
	}
	var recipe struct {
		Options struct {
			Scenarios []string
			Samples   int
			Suite     string
		}
	}
	if e = json.Unmarshal(method.Recipe, &recipe); e != nil || len(recipe.Options.Scenarios) != 1 || recipe.Options.Samples != 3 || recipe.Options.Suite != "" {
		t.Fatal("recipe includes unrelated scheduling", e)
	}
	b.Manifest.ID = "another-pass"
	b.Manifest.Lock.Options.Scenarios = []string{"steady", "first-call"}
	b.Manifest.Lock.Options.Suite = "another-corpus"
	b.Manifest.Lock.Options.ScenarioSamples["compile"] = 500
	other, e := siteMethod([]experiment.Bundle{b}, "another-pass", "engine", "fixture/a", "steady", "memory", "process.rss", "median_bytes", map[string]bool{"memory-0": true})
	x, _ := siteID(method)
	y, _ := siteID(other)
	if e != nil || x != y {
		t.Fatal("pass/scheduling identity split reusable method", e)
	}
	b.Trials[0].Observations[0].CollectorVersion = "2"
	other, e = siteMethod([]experiment.Bundle{b}, "another-pass", "engine", "fixture/a", "steady", "memory", "process.rss", "median_bytes", map[string]bool{"memory-0": true})
	y, _ = siteID(other)
	if e != nil || x == y {
		t.Fatal("collector versions collapsed", e)
	}
	_, e = siteMethod([]experiment.Bundle{b}, "another-pass", "engine", "fixture/a", "steady", "timing", "time.wall", "median_ns_per_operation", nil)
	if e == nil {
		t.Fatal("source-profile mismatch accepted")
	}
}

func TestSiteMethodMissingContextAndTrialScope(t *testing.T) {
	b := siteFixture().Bundle
	m, e := siteMethod([]experiment.Bundle{b}, "missing", "engine", "fixture/a", "steady", "memory", "process.rss", "median_bytes", nil)
	if e != nil || m.Status != "unavailable" || m.RecipeSHA256 != "" || len(m.Recipe) != 0 {
		t.Fatal("manufactured missing recipe")
	}
	m, e = siteMethod([]experiment.Bundle{b}, b.Manifest.ID, "engine", "fixture/a", "steady", "timing", "time.wall", "median_ns_per_operation", nil)
	if e != nil || m.CollectorStatus != "not_recorded" || len(m.Observations) != 0 {
		t.Fatal("inferred a timing collector")
	}
	_, e = siteMethod([]experiment.Bundle{b, b}, b.Manifest.ID, "engine", "fixture/a", "steady", "timing", "time.wall", "median_ns_per_operation", nil)
	if e == nil {
		t.Fatal("duplicate pass identity accepted")
	}
}
