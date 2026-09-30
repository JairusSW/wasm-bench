package sourcebuild

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/wasmbench/wasmbench/agent"
)

func validateStepResources(s StepResult, p *agent.ResourcePolicy, profile string) error {
	if p == nil {
		if s.Resources != nil {
			return fmt.Errorf("unexpected source resource evidence")
		}
		return nil
	}
	r := s.Resources
	if r == nil || r.Isolation == nil || r.WallNS != s.ElapsedNS || r.OOM || r.CleanupError != "" {
		return fmt.Errorf("invalid source resource evidence")
	}
	if err := agent.ValidateNUMAIsolation(*p, r.Isolation, "tool_exit_before_cleanup"); err != nil {
		return err
	}
	if p.CgroupParent == "" {
		if r.Isolation.Mode != "uncontrolled" {
			return fmt.Errorf("unexpected source isolation mode")
		}
	} else {
		if r.Isolation.Mode != "cgroup_v2_at_spawn" || filepath.Dir(r.Isolation.Path) != filepath.Clean(p.CgroupParent) {
			return fmt.Errorf("source tool was not isolated at spawn")
		}
		want := map[string]string{}
		if p.MemoryMaxBytes > 0 {
			want["memory.max"] = strconv.FormatUint(p.MemoryMaxBytes, 10)
			want["memory.oom.group"] = "1"
		}
		if p.DisableSwap {
			want["memory.swap.max"] = "0"
		}
		if p.CPUQuotaUS > 0 {
			want["cpu.max"] = fmt.Sprintf("%d 100000", p.CPUQuotaUS)
		}
		if p.PidsMax > 0 {
			want["pids.max"] = strconv.FormatUint(p.PidsMax, 10)
		}
		for key, value := range want {
			if r.Isolation.Effective[key] != value {
				return fmt.Errorf("effective source resource policy differs: %s", key)
			}
		}
		if p.CPUs != "" {
			requested, err := normalizedCPUList(p.CPUs)
			actual, actualErr := normalizedCPUList(r.Isolation.Effective["cpuset.cpus"])
			if err != nil || actualErr != nil || requested != actual {
				return fmt.Errorf("effective source resource policy differs: cpuset.cpus")
			}
		}
	}
	if profile != "memory" {
		if len(r.Observations) != 0 {
			return fmt.Errorf("memory collection in non-memory source pass")
		}
		return nil
	}
	if len(r.Observations) != 2 {
		return fmt.Errorf("missing source cgroup memory observations")
	}
	seen := map[string]bool{}
	for _, o := range r.Observations {
		quality := "boundary_snapshot_only"
		if o.Metric == "source.build.cgroup.peak" {
			quality = "kernel_accounted_peak"
		} else if o.Metric != "source.build.cgroup.current" {
			return fmt.Errorf("unknown source memory metric")
		}
		if seen[o.Metric] || o.DefinitionVersion != 1 || o.Unit != "bytes" || o.Scope != "tool_step_cgroup" || o.Phase != "tool_step/post_wait" || o.Collector != "cgroup_v2" || o.CollectorVersion != "1" || o.Quality != quality || o.Profile != "memory" || o.Denominator != "tool_step_cgroup" {
			return fmt.Errorf("invalid source memory metric identity")
		}
		seen[o.Metric] = true
		if o.Status == "available" {
			if o.Value == nil || *o.Value < 0 || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || math.Trunc(*o.Value) != *o.Value || o.Reason != "" {
				return fmt.Errorf("invalid source memory value")
			}
		} else if o.Status != "unavailable" || o.Value != nil || o.Reason == "" {
			return fmt.Errorf("invalid source memory availability")
		}
	}
	return nil
}

// Normalize intervals without expanding them: receipt validation must remain
// bounded by the input length even for enormous CPU ranges.
func normalizedCPUList(s string) (string, error) {
	type interval struct{ lo, hi uint64 }
	var spans []interval
	for _, item := range strings.Split(s, ",") {
		parts := strings.Split(item, "-")
		if len(parts) > 2 {
			return "", fmt.Errorf("invalid CPU list")
		}
		lo, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			return "", err
		}
		hi := lo
		if len(parts) == 2 {
			hi, err = strconv.ParseUint(parts[1], 10, 32)
			if err != nil || hi < lo {
				return "", fmt.Errorf("invalid CPU range")
			}
		}
		spans = append(spans, interval{lo, hi})
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].lo < spans[j].lo })
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) == 0 || span.lo > merged[len(merged)-1].hi+1 {
			merged = append(merged, span)
		} else if span.hi > merged[len(merged)-1].hi {
			merged[len(merged)-1].hi = span.hi
		}
	}
	var parts []string
	for _, span := range merged {
		parts = append(parts, fmt.Sprintf("%d-%d", span.lo, span.hi))
	}
	return strings.Join(parts, ","), nil
}

func maximumStepPeak(steps []StepResult) *float64 {
	var peak float64
	if len(steps) == 0 {
		return nil
	}
	for _, s := range steps {
		if s.Resources == nil {
			return nil
		}
		found := false
		for _, o := range s.Resources.Observations {
			if o.Metric == "source.build.cgroup.peak" {
				if o.Status != "available" || o.Value == nil {
					return nil
				}
				found = true
				if *o.Value > peak {
					peak = *o.Value
				}
			}
		}
		if !found {
			return nil
		}
	}
	return &peak
}

func buildFailureStatus(r Result) string {
	for _, s := range r.Steps {
		if s.Resources != nil && s.Resources.OOM {
			return "oom"
		}
	}
	for _, s := range r.Steps {
		if s.ContextError == "deadline_exceeded" {
			return "timeout"
		}
		if s.ContextError == "canceled" {
			return "canceled"
		}
	}
	return "build_failed"
}
