package analysis

import (
	"encoding/json"
	"strconv"

	"github.com/wasmbench/wasmbench/experiment"
)

const V8StackVersion = "v8-isolate-sample-tree-v1"

// v8CPUStack counts occurrences in the inspector samples array. It does not
// infer CPU time from timeDeltas, hitCount, duration, or the requested interval.
func v8CPUStack(t experiment.Trial) CPUStackProfile {
	p := t.CPUProfile
	out := CPUStackProfile{Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, TrialStatus: t.Status, ProfileSHA256: p.SHA256, ProfileExtension: p.Extension(), Version: V8StackVersion, Decoder: V8StackVersion, WeightUnit: "isolate_samples", Status: "invalid", Nodes: []CPUStackNode{}}
	fail := func(status, reason string) CPUStackProfile {
		out.Status = status
		out.Reason = reason
		out.Nodes = []CPUStackNode{}
		return out
	}
	if err := p.Validate(p.ModuleSHA256); err != nil {
		return fail("invalid", err.Error())
	}
	if p.Status != "collected" {
		return fail(p.Status, p.Reason)
	}
	var raw struct {
		Nodes []struct {
			ID       int64   `json:"id"`
			Children []int64 `json:"children"`
			Frame    *struct {
				Name   string `json:"functionName"`
				URL    string `json:"url"`
				Script string `json:"scriptId"`
				Line   *int64 `json:"lineNumber"`
				Column *int64 `json:"columnNumber"`
			} `json:"callFrame"`
		} `json:"nodes"`
		Samples []int64 `json:"samples"`
	}
	if err := json.Unmarshal(p.Data, &raw); err != nil {
		return fail("invalid", "invalid native node/sample types")
	}
	if raw.Samples == nil {
		return fail("unsupported", "samples array absent; hitCount and time deltas are not substituted")
	}
	if len(raw.Nodes) == 0 {
		return fail("invalid", "profile has no root node")
	}
	if len(raw.Nodes) > 100000 || len(raw.Samples) > 1000000 {
		return fail("budget_exceeded", "profile exceeds 100000 nodes or 1000000 recorded samples")
	}
	index := make(map[int64]int, len(raw.Nodes))
	parents := make([]int, len(raw.Nodes))
	labels := 0
	for i, n := range raw.Nodes {
		if n.ID <= 0 || n.ID > 9007199254740991 || n.Frame == nil {
			return fail("invalid", "invalid node identity or missing frame")
		}
		if _, ok := index[n.ID]; ok {
			return fail("invalid", "duplicate node identity")
		}
		index[n.ID] = i
		parents[i] = -1
		f := n.Frame
		if f.Line == nil || f.Column == nil || *f.Line < -1 || *f.Line >= 9007199254740991 || *f.Column < -1 || *f.Column >= 9007199254740991 {
			return fail("invalid", "invalid zero-based source coordinates")
		}
		labels += len(f.Name) + len(f.URL) + len(f.Script)
		if len(f.Name) > 4096 || len(f.URL) > 4096 || len(f.Script) > 4096 || labels > 8<<20 {
			return fail("budget_exceeded", "profile label budget exceeded")
		}
	}
	edges := 0
	for i, n := range raw.Nodes {
		for _, id := range n.Children {
			edges++
			if edges > 100000 {
				return fail("budget_exceeded", "profile edge budget exceeded")
			}
			child, ok := index[id]
			if !ok || child == i {
				return fail("invalid", "unknown or self-referencing child")
			}
			if parents[child] != -1 {
				return fail("invalid", "duplicate edge or multiple parents")
			}
			parents[child] = i
		}
	}
	root := -1
	for i, parent := range parents {
		if parent == -1 {
			if root != -1 {
				return fail("invalid", "multiple roots")
			}
			root = i
		}
	}
	if root < 0 {
		return fail("invalid", "cyclic profile has no root")
	}
	if root != 0 {
		return fail("invalid", "first node is not the root")
	}
	type visit struct{ index, depth int }
	pending := []visit{{root, 0}}
	order := make([]int, 0, len(raw.Nodes))
	seen := make([]bool, len(raw.Nodes))
	for len(pending) > 0 {
		v := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if v.depth > 1024 {
			return fail("budget_exceeded", "profile depth exceeds 1024")
		}
		if seen[v.index] {
			return fail("invalid", "cyclic profile")
		}
		seen[v.index] = true
		order = append(order, v.index)
		children := raw.Nodes[v.index].Children
		for i := len(children) - 1; i >= 0; i-- {
			pending = append(pending, visit{index[children[i]], v.depth + 1})
		}
	}
	if len(order) != len(raw.Nodes) {
		return fail("invalid", "disconnected or cyclic nodes")
	}
	self := make([]uint64, len(raw.Nodes))
	total := make([]uint64, len(raw.Nodes))
	for _, id := range raw.Samples {
		i, ok := index[id]
		if !ok {
			return fail("invalid", "sample references unknown node")
		}
		self[i]++
		total[i]++
	}
	for i := len(order) - 1; i >= 0; i-- {
		n := order[i]
		if parents[n] >= 0 {
			total[parents[n]] += total[n]
		}
	}
	outputID := make([]int, len(raw.Nodes))
	for i, n := range order {
		outputID[n] = i
	}
	for _, i := range order {
		n := raw.Nodes[i]
		f := n.Frame
		parent := -1
		if parents[i] >= 0 {
			parent = outputID[parents[i]]
		}
		name := f.Name
		if name == "" {
			name = "[anonymous or unresolved]"
		}
		out.Nodes = append(out.Nodes, CPUStackNode{ID: outputID[i], Parent: parent, Name: name, File: f.URL, Line: *f.Line + 1, Location: "node=" + strconv.FormatInt(n.ID, 10) + ";script=" + f.Script + ";column0=" + strconv.FormatInt(*f.Column, 10), TotalWeight: strconv.FormatUint(total[i], 10), SelfWeight: strconv.FormatUint(self[i], 10)})
	}
	out.SampleRecords = len(raw.Samples)
	out.TotalWeight = strconv.Itoa(len(raw.Samples))
	out.Status = "available"
	if len(raw.Samples) == 0 {
		out.Status = "empty"
		out.Reason = "no recorded isolate samples; not proof of zero CPU work"
	}
	return out
}
