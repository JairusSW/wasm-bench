package analysis

import (
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/sourcebuild"
	"testing"
)

func TestSourceHostBaselineExcludesAllMeasurementProfiles(t *testing.T) {
	for _, profile := range []string{"timing", "cpu", "memory"} {
		t.Run(profile, func(t *testing.T) {
			h := agent.Host{OS: "linux", Arch: "arm64", Hostname: "worker", CPUs: 4, PageSize: 4096, Environment: map[string]string{}, Policy: map[string]string{}}
			p := &agent.HostPolicy{Version: agent.HostPolicyVersion, Expected: h}
			start := agent.CheckHostPolicy(p, h, "before_run_preparation")
			changed := h
			changed.CPUs++
			end := agent.CheckHostPolicy(p, changed, "after_trials_before_seal")
			b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Profile: profile, HostPolicy: p, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}, {Recipe: sourcebuild.Recipe{ID: "b"}}}}, Host: h, HostEnd: &changed, HostStartCheck: &start, HostEndCheck: &end, Publication: "prohibited_host_baseline_mismatch"}
			if err := b.ValidateHostEvidence(); err != nil {
				t.Fatal(err)
			}
			value := int64(100)
			peak := float64(100)
			for block := 0; block < 3; block++ {
				for variant := 0; variant < 2; variant++ {
					b.Trials = append(b.Trials, sourcebuild.BuildTrial{Block: block, Variant: variant, Status: "ok", ToolWallNS: &value, ToolCPUNS: &value, MemoryPeakBytes: &peak})
				}
			}
			r := SummarizeSourceBuilds(b)
			for _, s := range r.Summaries {
				if s.Median != nil || s.MedianBytes != nil || s.SuccessfulBuilds != 0 || s.Outcomes["host_policy_mismatch"] != 3 {
					t.Fatal(s)
				}
			}
			if r.Comparisons[0].Ratio != nil {
				t.Fatal("mismatched host ratio published")
			}
			if b.Trials[0].Status != "ok" || *b.Trials[0].ToolWallNS != 100 {
				t.Fatal("raw evidence mutated")
			}
		})
	}
}

func TestSourcePartitionExcludesMissingEvidence(t *testing.T) {
	value := int64(100)
	b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{RequireIsolatedCPUPartition: true, Resources: &agent.ResourcePolicy{CgroupParent: "/partition", CPUs: "2"}, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}}}, Trials: []sourcebuild.BuildTrial{{Status: "ok", ToolWallNS: &value}}}
	r := SummarizeSourceBuilds(b)
	if r.Summaries[0].SuccessfulBuilds != 0 || r.Summaries[0].Median != nil || b.Trials[0].Status != "ok" {
		t.Fatal("partition failure used for source measurements")
	}
}

func TestSourceIRQExcludesAllProfilesWithoutChangingRawResults(t *testing.T) {
	for _, profile := range []string{"timing", "cpu", "memory"} {
		value, peak := int64(100), float64(200)
		b := sourcebuild.BuildBenchmark{Config: sourcebuild.BenchmarkConfig{Profile: profile, RequireIRQAffinity: true, Resources: &agent.ResourcePolicy{CgroupParent: "/cg", CPUs: "2"}, Variants: []sourcebuild.Lock{{Recipe: sourcebuild.Recipe{ID: "a"}}, {Recipe: sourcebuild.Recipe{ID: "b"}}}}}
		for block := 0; block < 3; block++ {
			for variant := 0; variant < 2; variant++ {
				b.Trials = append(b.Trials, sourcebuild.BuildTrial{Block: block, Variant: variant, Status: "ok", CPUStatus: "available", MemoryStatus: "available", ToolWallNS: &value, ToolCPUNS: &value, MemoryPeakBytes: &peak})
			}
		}
		r := SummarizeSourceBuilds(b)
		for _, s := range r.Summaries {
			if s.SuccessfulBuilds != 0 || s.Median != nil || s.MedianBytes != nil || s.Outcomes["host_policy_mismatch"] != 3 {
				t.Fatal(profile, s)
			}
		}
		if r.Comparisons[0].Ratio != nil || b.Trials[0].Status != "ok" || *b.Trials[0].ToolWallNS != 100 || *b.Trials[0].MemoryPeakBytes != 200 {
			t.Fatal("IRQ exclusion published estimates or changed raw evidence")
		}
	}
}
