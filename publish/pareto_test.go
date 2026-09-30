package publish

import (
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func TestParetoKeepsCoverageZerosTiesAndSeparateStages(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{Options: experiment.Options{Scenarios: []string{"compile", "instantiate"}}, Workloads: []protocol.Workload{{ID: "w"}}, Runtimes: []experiment.Runtime{{ID: "fast"}, {ID: "small"}, {ID: "dominated"}, {ID: "tie"}, {ID: "missing"}, {ID: "diagnostic"}}}}}
	var timing []analysis.Summary
	var memory []MemoryStage
	for _, item := range []struct {
		id        string
		time, rss float64
	}{{"fast", 0, 100}, {"small", 100, 0}, {"dominated", 100, 100}, {"tie", 0, 100}, {"diagnostic", 0, 0}} {
		timing = append(timing, analysis.Summary{Runtime: item.id, Workload: "w", Scenario: "compile", LatencyStatus: "timing_pass", Launches: 1, Median: protocol.Value(item.time)})
		memory = append(memory, MemoryStage{Runtime: item.id, Workload: "w", Scenario: "compile", Metric: "process.peak_rss", Launches: 1, Median: protocol.Value(item.rss), Trials: []string{"same-id"}})
	}
	timing[len(timing)-1].LatencyStatus = "diagnostic_pass"
	points := paretoPoints(b, timing, memory)
	if len(points) != 12 {
		t.Fatal("lost missing configurations/stages", len(points))
	}
	byID := map[string]ParetoPoint{}
	for _, p := range points {
		if p.Scenario == "compile" {
			byID[p.Runtime] = p
		} else if p.Status == "available" {
			t.Fatal("mixed stages", p)
		}
	}
	for _, id := range []string{"fast", "small", "tie"} {
		if !byID[id].Frontier {
			t.Fatal("zero/tie mishandled", byID[id])
		}
	}
	if byID["dominated"].Frontier || !reflect.DeepEqual(byID["dominated"].DominatedBy, []string{"fast", "small", "tie"}) {
		t.Fatal(byID["dominated"])
	}
	if byID["missing"].Status != "unavailable" || byID["diagnostic"].Time != nil || byID["diagnostic"].Frontier {
		t.Fatal("unavailable became measured", byID)
	}
	if byID["fast"].TimeLow != nil || byID["fast"].RSSLow != nil || !reflect.DeepEqual(byID["fast"].MemoryTrials, []string{"same-id"}) {
		t.Fatal("lost provenance or invented interval")
	}
}

func TestParetoRequiresMatchedRSSNotAllocatorOrInvalidValues(t *testing.T) {
	b := experiment.Bundle{Manifest: experiment.Manifest{Lock: experiment.Lock{Options: experiment.Options{Scenarios: []string{"compile"}}, Workloads: []protocol.Workload{{ID: "w"}}, Runtimes: []experiment.Runtime{{ID: "r"}}}}}
	timing := []analysis.Summary{{Runtime: "r", Workload: "w", Scenario: "compile", LatencyStatus: "timing_pass", Launches: 3, Median: protocol.Value(20), CILow: protocol.Value(10), CIHigh: protocol.Value(30)}}
	memory := []MemoryStage{{Runtime: "r", Workload: "w", Scenario: "compile", Metric: "host.alloc.bytes", Launches: 3, Median: protocol.Value(10)}}
	if p := paretoPoints(b, timing, memory)[0]; p.RSS != nil || p.Status != "unavailable" {
		t.Fatal("allocator mistaken for RSS", p)
	}
	memory[0].Metric = "process.peak_rss"
	for _, value := range []float64{-1, math.NaN(), math.Inf(1)} {
		memory[0].Median = protocol.Value(value)
		if paretoPoints(b, timing, memory)[0].Status != "unavailable" {
			t.Fatal("invalid footprint accepted")
		}
	}
	memory[0].Median, memory[0].Low, memory[0].High = protocol.Value(40), protocol.Value(35), protocol.Value(45)
	p := paretoPoints(b, timing, memory)[0]
	if p.Status != "available" || *p.TimeLow != 10 || *p.RSSHigh != 45 || p.TimeLaunches != 3 || p.MemoryLaunches != 3 {
		t.Fatal("independent uncertainty lost", p)
	}
	timing[0].Launches = 0
	if paretoPoints(b, timing, memory)[0].Status != "unavailable" {
		t.Fatal("zero launches accepted")
	}
}

func TestParetoReportRecomputedFromSeparateRawPasses(t *testing.T) {
	primary, _ := aggregateBundle(t, false)
	b, err := experiment.Load(primary)
	if err != nil {
		t.Fatal(err)
	}
	memory := t.TempDir()
	if err := os.Mkdir(filepath.Join(memory, "trials"), 0755); err != nil {
		t.Fatal(err)
	}
	b.Manifest.ID = "pareto-memory"
	b.Manifest.Lock.Options.Profile = "memory"
	if err := experiment.WriteJSON(filepath.Join(memory, "manifest.json"), b.Manifest); err != nil {
		t.Fatal(err)
	}
	for _, tr := range b.Trials {
		tr.Profile = "memory"
		tr.Samples = nil
		value := float64(100)
		if tr.Runtime == "candidate" {
			value = 50
		}
		tr.Observations = []protocol.Observation{{Metric: "process.peak_rss", DefinitionVersion: 1, Unit: "bytes", Scope: "adapter_process", Phase: "compile/process_lifetime", Collector: "wait4_rusage", CollectorVersion: "1", Quality: "kernel_accounted_peak", Profile: "memory", Status: "available", Denominator: "process", Value: protocol.Value(value)}}
		if err := experiment.WriteJSON(filepath.Join(memory, "trials", tr.ID+".json"), tr); err != nil {
			t.Fatal(err)
		}
	}
	if err := experiment.Seal(memory); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "report")
	if err := ReportWithMemory(primary, memory, out); err != nil {
		t.Fatal(err)
	}
	var d Dataset
	if err := experiment.ReadJSON(filepath.Join(out, "data.json"), &d); err != nil {
		t.Fatal(err)
	}
	if d.ParetoVersion != ParetoVersion || len(d.Pareto) != 2 {
		t.Fatal("missing versioned points", d.Pareto)
	}
	for _, p := range d.Pareto {
		if !p.Frontier || p.TimeLaunches != 3 || p.MemoryLaunches != 3 || len(p.MemoryTrials) != 3 || p.TimeLow == nil || p.RSSLow == nil {
			t.Fatal("lost trade-off uncertainty/provenance", p)
		}
	}
	// A resealed change to the trade-off classification is still rejected by
	// recomputing from the independent raw inputs, not trusting derived JSON.
	d.Pareto[0].Frontier = false
	if err := os.Remove(filepath.Join(out, "data.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.WriteJSON(filepath.Join(out, "data.json"), d); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(out, "checksums.json")); err != nil {
		t.Fatal(err)
	}
	if err := experiment.Seal(out); err != nil {
		t.Fatal(err)
	}
	if err := VerifyReport(out); err == nil || !strings.Contains(err.Error(), "dataset differs") {
		t.Fatal("trusted altered frontier", err)
	}
}
