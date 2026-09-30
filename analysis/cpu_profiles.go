package analysis

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"

	"github.com/google/pprof/profile"
	"github.com/wasmbench/wasmbench/experiment"
)

const CPUStackVersion = "pprof-cpu-stack-tree-v1"
const PprofDecoderVersion = "github.com/google/pprof@v0.0.0-20251114195745-4902fdda35c8"

type CPUStackNode struct {
	ID          int    `json:"id"`
	Parent      int    `json:"parent"`
	Name        string `json:"name"`
	File        string `json:"file,omitempty"`
	Line        int64  `json:"line,omitempty"`
	Address     string `json:"address,omitempty"`
	Location    string `json:"location,omitempty"`
	TotalNS     string `json:"total_ns"`
	SelfNS      string `json:"self_ns"`
	TotalWeight string `json:"total_weight,omitempty"`
	SelfWeight  string `json:"self_weight,omitempty"`
}

type CPUStackProfile struct {
	Trial            string         `json:"trial"`
	Runtime          string         `json:"runtime"`
	Workload         string         `json:"workload"`
	TrialStatus      string         `json:"trial_status"`
	ProfileSHA256    string         `json:"profile_sha256"`
	ProfileExtension string         `json:"profile_extension"`
	Version          string         `json:"version"`
	Decoder          string         `json:"decoder"`
	Status           string         `json:"status"`
	Reason           string         `json:"reason,omitempty"`
	SampleRecords    int            `json:"sample_records"`
	DurationNS       string         `json:"duration_ns"`
	TotalNS          string         `json:"total_ns"`
	WeightUnit       string         `json:"weight_unit,omitempty"`
	TotalWeight      string         `json:"total_weight,omitempty"`
	Nodes            []CPUStackNode `json:"nodes"`
}

// CPUStacks derives a per-trial call tree, never combines independent profiles
// or filters failed-trial evidence. Raw sample labels remain in the pprof file;
// this view aggregates all labels, not a thread/goroutine-specific attribution.
func CPUStacks(b experiment.Bundle) []CPUStackProfile {
	var out []CPUStackProfile
	for _, t := range b.Trials {
		if t.CPUProfile != nil {
			out = append(out, cpuStack(t))
		}
	}
	return out
}

type stackTree struct {
	frame       CPUStackNode
	total, self uint64
	children    map[string]*stackTree
}

func cpuStack(t experiment.Trial) CPUStackProfile {
	if t.CPUProfile.Format == "v8-cpuprofile-json" {
		return v8CPUStack(t)
	}
	p := t.CPUProfile
	extension := p.Extension()
	out := CPUStackProfile{Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, TrialStatus: t.Status, ProfileSHA256: p.SHA256, Version: CPUStackVersion, Decoder: PprofDecoderVersion, Status: "invalid", Nodes: []CPUStackNode{}}
	fail := func(status, reason string) CPUStackProfile {
		out.Status, out.Reason = status, reason
		out.Nodes = []CPUStackNode{}
		return out
	}
	out.ProfileExtension = extension
	if err := p.Validate(p.ModuleSHA256); err != nil {
		return fail("invalid", err.Error())
	}
	if p.Status != "collected" {
		return fail(p.Status, p.Reason)
	}
	if p.Format != "pprof-gzip" {
		return fail("unsupported", "V8 isolate profile remains in its native format; time deltas are not treated as Go process CPU nanoseconds")
	}
	z, err := gzip.NewReader(bytes.NewReader(p.Data))
	if err != nil {
		return fail("invalid", err.Error())
	}
	raw, err := io.ReadAll(io.LimitReader(z, (64<<20)+1))
	z.Close()
	if err != nil || len(raw) > 64<<20 {
		return fail("invalid", "invalid or oversized decoded profile")
	}
	parsed, err := profile.ParseUncompressed(raw)
	if err != nil {
		return fail("invalid", err.Error())
	}
	if err = parsed.CheckValid(); err != nil {
		return fail("invalid", err.Error())
	}
	if len(parsed.Sample) > 100000 || len(parsed.Location) > 100000 || len(parsed.Function) > 100000 {
		return fail("budget_exceeded", "profile exceeds 100000 sample/location/function analysis budget; raw evidence retained")
	}
	index := -1
	for i, typ := range parsed.SampleType {
		if typ.Type == "cpu" && typ.Unit == "nanoseconds" {
			if index >= 0 {
				return fail("unsupported", "ambiguous cpu/nanoseconds sample types")
			}
			index = i
		}
	}
	if index < 0 {
		return fail("unsupported", "no cpu/nanoseconds sample type; no unit conversion inferred")
	}
	out.SampleRecords = len(parsed.Sample)
	if parsed.DurationNanos < 0 {
		return fail("invalid", "negative profile duration")
	}
	out.DurationNS = strconv.FormatInt(parsed.DurationNanos, 10)
	root := &stackTree{frame: CPUStackNode{Name: "All sampled CPU"}, children: map[string]*stackTree{}}
	nodes, steps, labelBytes := 1, 0, 0
	for _, sample := range parsed.Sample {
		value := sample.Value[index]
		if value < 0 {
			return fail("unsupported", "negative sample weights cannot form a CPU flamegraph")
		}
		weight := uint64(value)
		if math.MaxUint64-root.total < weight {
			return fail("invalid", "sample total overflows uint64")
		}
		root.total += weight
		cursor := root
		depth := 0
		add := func(key string, frame CPUStackNode) bool {
			depth++
			steps++
			if depth > 1024 || steps > 1000000 {
				return false
			}
			child := cursor.children[key]
			if child == nil {
				nodes++
				labelBytes += len(frame.Name) + len(frame.File) + len(frame.Address) + len(frame.Location)
				if nodes > 100000 || labelBytes > 8<<20 || len(frame.Name) > 4096 || len(frame.File) > 4096 {
					return false
				}
				child = &stackTree{frame: frame, children: map[string]*stackTree{}}
				cursor.children[key] = child
			}
			child.total += weight
			cursor = child
			return true
		}
		// pprof locations and inlined lines are leaf-first. Reverse both to
		// build caller -> callee paths without merging recursive occurrences.
		for li := len(sample.Location) - 1; li >= 0; li-- {
			loc := sample.Location[li]
			base := CPUStackNode{Location: strconv.FormatUint(loc.ID, 10), Address: fmt.Sprintf("0x%x", loc.Address)}
			if len(loc.Line) == 0 {
				base.Name = "[unresolved location " + base.Location + "]"
				if !add(base.Location+"/unknown", base) {
					return fail("budget_exceeded", "stack tree depth/node/expansion budget exceeded; raw evidence retained")
				}
			}
			for j := len(loc.Line) - 1; j >= 0; j-- {
				line := loc.Line[j]
				frame := base
				frame.Line = line.Line
				frame.Name = "[unresolved function]"
				if line.Function != nil {
					frame.Name = line.Function.Name
					frame.File = line.Function.Filename
					if frame.Name == "" {
						frame.Name = "[unnamed function]"
					}
				}
				if !add(base.Location+"/"+strconv.Itoa(j), frame) {
					return fail("budget_exceeded", "stack tree depth/node/expansion budget exceeded; raw evidence retained")
				}
			}
		}
		if len(sample.Location) == 0 {
			if !add("missing", CPUStackNode{Name: "[unresolved stack]"}) {
				return fail("budget_exceeded", "stack tree budget exceeded")
			}
		}
		cursor.self += weight
	}
	var emit func(*stackTree, int)
	emit = func(n *stackTree, parent int) {
		frame := n.frame
		frame.ID = len(out.Nodes)
		frame.Parent = parent
		frame.TotalNS = strconv.FormatUint(n.total, 10)
		frame.SelfNS = strconv.FormatUint(n.self, 10)
		out.Nodes = append(out.Nodes, frame)
		keys := make([]string, 0, len(n.children))
		for key := range n.children {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			emit(n.children[key], frame.ID)
		}
	}
	emit(root, -1)
	out.TotalNS = strconv.FormatUint(root.total, 10)
	out.Status = "available"
	if root.total == 0 {
		out.Status = "empty"
		out.Reason = "profile contains no positive CPU sample weight; not proof of zero CPU work"
	}
	return out
}
