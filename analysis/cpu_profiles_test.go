package analysis

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func profileTrial(t *testing.T, p *profile.Profile) experiment.Trial {
	t.Helper()
	var buf bytes.Buffer
	if err := p.Write(&buf); err != nil {
		t.Fatal(err)
	}
	digest := corpus.Hash(buf.Bytes())
	return experiment.Trial{ID: "trial", Runtime: "runtime", Workload: "work", Profile: "profiling", Status: "error", CPUProfile: &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "pprof-gzip", Collector: "runtime/pprof", CollectorVersion: "test", Scope: "adapter_process_go_cpu", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: buf.Bytes()}}
}

func TestV8ProfileIsNotInterpretedAsGoCPU(t *testing.T) {
	data := []byte(`{"nodes":[],"startTime":0,"endTime":10}`)
	digest := corpus.Hash(data)
	p := &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "v8-cpuprofile-json", Collector: "node:inspector/Profiler", CollectorVersion: "test", Scope: "adapter_v8_isolate_sampled_stacks", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: data}
	out := cpuStack(experiment.Trial{CPUProfile: p})
	if out.Status != "unsupported" || out.TotalNS != "" || len(out.Nodes) != 0 || out.ProfileExtension != ".cpuprofile" {
		t.Fatal(out)
	}
}

func TestCPUStackExactWeightsInlineRecursionAndUnknown(t *testing.T) {
	f1 := &profile.Function{ID: 1, Name: "caller", Filename: "caller.go"}
	f2 := &profile.Function{ID: 2, Name: "outer"}
	f3 := &profile.Function{ID: 3, Name: "<script>inner</script>"}
	l1 := &profile.Location{ID: 1, Line: []profile.Line{{Function: f1, Line: 3}}}
	l2 := &profile.Location{ID: 2, Address: math.MaxUint64, Line: []profile.Line{{Function: f3, Line: 9}, {Function: f2, Line: 6}}}
	p := &profile.Profile{SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}}, DurationNanos: 99, Function: []*profile.Function{f1, f2, f3}, Location: []*profile.Location{l1, l2}, Sample: []*profile.Sample{
		{Location: []*profile.Location{l2, l1}, Value: []int64{9007199254740993}},
		{Location: []*profile.Location{l1, l1}, Value: []int64{7}},
		{Value: []int64{3}},
	}}
	trial := profileTrial(t, p)
	original := append([]byte(nil), trial.CPUProfile.Data...)
	got := cpuStack(trial)
	if got.Status != "available" || got.TotalNS != "9007199254741003" || got.TrialStatus != "error" || !bytes.Equal(original, trial.CPUProfile.Data) {
		t.Fatal(got)
	}
	paths := map[int]string{}
	for _, n := range got.Nodes {
		paths[n.ID] = paths[n.Parent] + "/" + n.Name
	}
	foundInline, foundRecursion, foundUnknown := false, false, false
	for _, n := range got.Nodes {
		if paths[n.ID] == "/All sampled CPU/caller/outer/<script>inner</script>" {
			foundInline = true
			if n.SelfNS != "9007199254740993" || n.Address != "0xffffffffffffffff" {
				t.Fatal(n)
			}
		}
		if paths[n.ID] == "/All sampled CPU/caller/caller" {
			foundRecursion = true
		}
		if n.Name == "[unresolved stack]" {
			foundUnknown = true
		}
		total, _ := strconv.ParseUint(n.TotalNS, 10, 64)
		self, _ := strconv.ParseUint(n.SelfNS, 10, 64)
		for _, child := range got.Nodes {
			if child.Parent == n.ID {
				v, _ := strconv.ParseUint(child.TotalNS, 10, 64)
				self += v
			}
		}
		if total != self {
			t.Fatal("tree weight not conserved", n)
		}
	}
	if !foundInline || !foundRecursion || !foundUnknown {
		t.Fatal(paths)
	}
}

func TestCPUStackUnavailableAndInvalidCases(t *testing.T) {
	for _, mode := range []string{"empty", "wrong-unit", "ambiguous", "negative", "overflow", "unknown", "deep", "long-name"} {
		t.Run(mode, func(t *testing.T) {
			p := &profile.Profile{SampleType: []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}}}
			want := "unsupported"
			switch mode {
			case "empty":
				want = "empty"
			case "wrong-unit":
				p.SampleType[0].Unit = "seconds"
			case "ambiguous":
				p.SampleType = append(p.SampleType, &profile.ValueType{Type: "cpu", Unit: "nanoseconds"})
			case "negative":
				p.Sample = []*profile.Sample{{Value: []int64{-1}}}
			case "overflow":
				want = "invalid"
				for i := 0; i < 3; i++ {
					p.Sample = append(p.Sample, &profile.Sample{Value: []int64{math.MaxInt64}})
				}
			case "unknown":
				want = "available"
				l := &profile.Location{ID: 1, Address: 123}
				p.Location = []*profile.Location{l}
				p.Sample = []*profile.Sample{{Location: []*profile.Location{l}, Value: []int64{1}}}
			case "deep":
				want = "budget_exceeded"
				l := &profile.Location{ID: 1}
				p.Location = []*profile.Location{l}
				s := &profile.Sample{Value: []int64{1}}
				for i := 0; i < 1025; i++ {
					s.Location = append(s.Location, l)
				}
				p.Sample = []*profile.Sample{s}
			case "long-name":
				want = "budget_exceeded"
				f := &profile.Function{ID: 1, Name: strings.Repeat("x", 4097)}
				l := &profile.Location{ID: 1, Line: []profile.Line{{Function: f}}}
				p.Function = []*profile.Function{f}
				p.Location = []*profile.Location{l}
				p.Sample = []*profile.Sample{{Location: []*profile.Location{l}, Value: []int64{1}}}
			}
			got := cpuStack(profileTrial(t, p))
			if got.Status != want {
				t.Fatal(got)
			}
			if want != "available" && want != "empty" && len(got.Nodes) != 0 {
				t.Fatal("partial tree leaked")
			}
		})
	}
}
