package analysis

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

func v8Trial(data []byte) experiment.Trial {
	digest := corpus.Hash(data)
	return experiment.Trial{Status: "error", CPUProfile: &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "v8-cpuprofile-json", Collector: "node:inspector/Profiler", CollectorVersion: "fixture", Scope: "adapter_v8_isolate_sampled_stacks", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: data}}
}
func v8Node(id int, children ...int) map[string]any {
	return map[string]any{"id": id, "children": children, "callFrame": map[string]any{"functionName": "recursive", "url": "wasm://fixture", "scriptId": "1", "lineNumber": -1, "columnNumber": 0}}
}
func TestV8SampleTree(t *testing.T) {
	nodes := []any{v8Node(1, 2, 4), v8Node(2, 3), v8Node(3), v8Node(4)}
	nodes[2].(map[string]any)["callFrame"].(map[string]any)["functionName"] = "<img src=x onerror=alert(1)>"
	raw, _ := json.Marshal(map[string]any{"nodes": nodes, "samples": []int{3, 3, 2, 4}, "startTime": 0, "endTime": 100, "timeDeltas": []int{1, 99, 0, 0}})
	trial := v8Trial(raw)
	before := append([]byte(nil), raw...)
	out := cpuStack(trial)
	if out.Status != "available" || out.TotalWeight != "4" || out.TotalNS != "" || out.DurationNS != "" || out.WeightUnit != "isolate_samples" || out.TrialStatus != "error" || !bytes.Equal(before, trial.CPUProfile.Data) {
		t.Fatal(out)
	}
	if len(out.Nodes) != 4 || out.Nodes[1].TotalWeight != "3" || out.Nodes[1].SelfWeight != "1" || out.Nodes[2].SelfWeight != "2" || out.Nodes[2].Parent != 1 {
		t.Fatal(out.Nodes)
	}
	for _, n := range out.Nodes {
		if n.TotalNS != "" || n.SelfNS != "" {
			t.Fatal("counts mislabeled as time")
		}
		total, _ := strconv.Atoi(n.TotalWeight)
		self, _ := strconv.Atoi(n.SelfWeight)
		for _, c := range out.Nodes {
			if c.Parent == n.ID {
				v, _ := strconv.Atoi(c.TotalWeight)
				self += v
			}
		}
		if total != self {
			t.Fatal("nonconserving tree", n)
		}
	}
}
func TestV8MalformedAndMissing(t *testing.T) {
	for _, mode := range []string{"missing", "empty", "duplicate", "unknown-child", "unknown-sample", "cycle", "disconnected-cycle", "multiple-parents", "duplicate-edge", "missing-frame", "bad-id", "label-budget", "deep"} {
		t.Run(mode, func(t *testing.T) {
			nodes := []any{v8Node(1, 2), v8Node(2)}
			raw := map[string]any{"nodes": nodes, "samples": []int{2}, "startTime": 0, "endTime": 1}
			want := "invalid"
			switch mode {
			case "missing":
				delete(raw, "samples")
				want = "unsupported"
			case "empty":
				raw["samples"] = []int{}
				want = "empty"
			case "duplicate":
				nodes[1] = v8Node(1)
			case "unknown-child":
				nodes[0] = v8Node(1, 9)
			case "unknown-sample":
				raw["samples"] = []int{9}
			case "cycle":
				nodes[1] = v8Node(2, 1)
			case "disconnected-cycle":
				raw["nodes"] = []any{v8Node(1), v8Node(2, 3), v8Node(3, 2)}
			case "multiple-parents":
				raw["nodes"] = []any{v8Node(1, 2, 3), v8Node(2, 3), v8Node(3)}
			case "duplicate-edge":
				nodes[0] = v8Node(1, 2, 2)
			case "missing-frame":
				delete(nodes[1].(map[string]any), "callFrame")
			case "bad-id":
				nodes[1].(map[string]any)["id"] = 1.5
			case "label-budget":
				nodes[1].(map[string]any)["callFrame"].(map[string]any)["functionName"] = strings.Repeat("a", 4097)
				want = "budget_exceeded"
			case "deep":
				nodes = nil
				for i := 1; i <= 1026; i++ {
					if i == 1026 {
						nodes = append(nodes, v8Node(i))
					} else {
						nodes = append(nodes, v8Node(i, i+1))
					}
				}
				raw["nodes"] = nodes
				want = "budget_exceeded"
			}
			data, _ := json.Marshal(raw)
			out := cpuStack(v8Trial(data))
			if out.Status != want {
				t.Fatal(mode, out)
			}
			if want != "empty" && len(out.Nodes) != 0 {
				t.Fatal("partial tree exposed")
			}
		})
	}
}

func TestV8MissingCoordinatesAndSampleBudget(t *testing.T) {
	n := v8Node(1)
	frame := n["callFrame"].(map[string]any)
	delete(frame, "lineNumber")
	data, _ := json.Marshal(map[string]any{"nodes": []any{n}, "samples": []int{}, "startTime": 0, "endTime": 1})
	if got := cpuStack(v8Trial(data)); got.Status != "invalid" {
		t.Fatal(got)
	}
	samples := make([]int, 1000001)
	for i := range samples {
		samples[i] = 1
	}
	data, _ = json.Marshal(map[string]any{"nodes": []any{v8Node(1)}, "samples": samples, "startTime": 0, "endTime": 1})
	if got := cpuStack(v8Trial(data)); got.Status != "budget_exceeded" || len(got.Nodes) != 0 {
		t.Fatal(got)
	}
}
