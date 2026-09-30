package analysis

import (
	"reflect"
	"testing"

	"github.com/wasmbench/wasmbench/sourcebuild"
)

func TestSourceBuildBlockStatistics(t *testing.T) {
	b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Blocks: 3, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}, {Recipe: sourcebuild.Recipe{ID: "b"}}}}}
	for block := -1; block < 3; block++ {
		for variant := 0; variant < 2; variant++ {
			v := int64((block + 1) * 10 * (variant + 1))
			if block < 0 {
				v = 1000000
			}
			b.Trials = append(b.Trials, sourcebuild.BuildTrial{Block: block, Variant: variant, Warmup: block < 0, Status: "ok", ToolWallNS: &v})
		}
	}
	r := SummarizeSourceBuilds(b)
	if r.Measurement.Name != "source.build.tool_wall" || *r.Summaries[0].Median != 20 || *r.Summaries[1].Median != 40 || r.Summaries[0].WarmupOutcomes["ok"] != 1 || r.Summaries[0].SuccessfulBuilds != 3 {
		t.Fatal(r)
	}
	p := r.Comparisons[0]
	if p.Pairs != 3 || *p.Ratio != 2 || *p.Low != 2 || *p.High != 2 {
		t.Fatal(p)
	}
	if !reflect.DeepEqual(r, SummarizeSourceBuilds(b)) {
		t.Fatal("non-deterministic intervals")
	}
	b.Trials[2].Status = "build_failed"
	b.Trials[2].ToolWallNS = nil
	r = SummarizeSourceBuilds(b)
	if r.Summaries[0].Outcomes["build_failed"] != 1 || r.Comparisons[0].Pairs != 2 || r.Comparisons[0].Low != nil {
		t.Fatal(r)
	}
	for i := range b.Trials {
		b.Trials[i].Status = "different_output"
		b.Trials[i].ToolWallNS = nil
	}
	r = SummarizeSourceBuilds(b)
	if r.Summaries[0].Median != nil || r.Comparisons[0].Ratio != nil {
		t.Fatal("failed build represented as zero", r)
	}
}

func TestSourceCPUStatisticsDoNotUseWallTime(t *testing.T) {
	b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Profile: "cpu", Blocks: 3, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}}}}
	for i := 0; i < 3; i++ {
		cpu, wall := int64(100*(i+1)), int64(10000)
		b.Trials = append(b.Trials, sourcebuild.BuildTrial{Block: i, Variant: 0, Status: "ok", CPUStatus: "available", ToolCPUNS: &cpu, ToolWallNS: &wall})
	}
	r := SummarizeSourceBuilds(b)
	if r.Measurement.Name != "source.build.wait_cpu" || *r.Summaries[0].Median != 200 {
		t.Fatal("reported wall time as CPU", r)
	}
	b.Trials[0].ToolCPUNS = nil
	b.Trials[0].CPUStatus = "unavailable"
	r = SummarizeSourceBuilds(b)
	if r.Summaries[0].MissingMeasurements != 1 || *r.Summaries[0].Median != 250 || r.Summaries[0].Low != nil {
		t.Fatal("unavailable CPU counted as zero", r)
	}
}

func TestSourceMemoryStatisticsUseBytes(t *testing.T) {
	b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Profile: "memory", Blocks: 3, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}}}}
	for i := -1; i < 3; i++ {
		peak, wall := float64(100*(i+1)), int64(999999)
		if i < 0 {
			peak = 999999
		}
		b.Trials = append(b.Trials, sourcebuild.BuildTrial{Block: i, Variant: 0, Warmup: i < 0, Status: "ok", MemoryStatus: "available", MemoryPeakBytes: &peak, ToolWallNS: &wall})
	}
	r := SummarizeSourceBuilds(b)
	s := r.Summaries[0]
	if r.Measurement.Name != "source.build.max_step_cgroup_peak" || s.Median != nil || s.Mean != nil || s.StdDev != nil || s.MedianBytes == nil || *s.MedianBytes != 200 {
		t.Fatal("incorrect memory units or warmup handling", r)
	}
	b.Trials[1].MemoryPeakBytes = nil
	b.Trials[1].MemoryStatus = "unavailable"
	r = SummarizeSourceBuilds(b)
	if r.Summaries[0].MissingMeasurements != 1 || *r.Summaries[0].MedianBytes != 250 || r.Summaries[0].Low != nil {
		t.Fatal("missing memory became zero", r)
	}
}

func TestSourceTimeoutOutcomesAreNotSamples(t *testing.T) {
	b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}}}}
	for _, status := range []string{"timeout", "canceled", "oom", "build_failed"} {
		b.Trials = append(b.Trials, sourcebuild.BuildTrial{Status: status, Variant: 0})
	}
	r := SummarizeSourceBuilds(b)
	s := r.Summaries[0]
	if s.SuccessfulBuilds != 0 || s.Median != nil || s.MedianBytes != nil || len(s.Outcomes) != 4 {
		t.Fatal("failures became samples", s)
	}
	for _, status := range []string{"timeout", "canceled", "oom", "build_failed"} {
		if s.Outcomes[status] != 1 {
			t.Fatal("lost failure category", s)
		}
	}
}
