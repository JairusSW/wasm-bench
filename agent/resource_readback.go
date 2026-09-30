package agent

import (
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/wasmbench/wasmbench/collectors"
)

const ResourceReadbackVersion = "requested-cgroup-leaf-readback-v2"

type ResourceVerification struct {
	Version   string            `json:"version"`
	Stage     string            `json:"stage"`
	Scope     string            `json:"scope"`
	Status    string            `json:"status"`
	Reason    string            `json:"reason,omitempty"`
	Effective map[string]string `json:"effective"`
}

// CheckResourceReadback compares requested leaf controls with a boundary read.
// It establishes neither CPU exclusivity nor ancestor limits or continuous
// enforcement. No zero/unlimited defaults are substituted for absent readback.
func CheckResourceReadback(p ResourcePolicy, effective map[string]string, stage string) ResourceVerification {
	v := ResourceVerification{Version: ResourceReadbackVersion, Stage: stage, Scope: "explicit_requested_leaf_controls_at_boundary_not_exclusive_or_ancestor_limits", Status: "not_requested", Effective: map[string]string{}}
	for k, value := range effective {
		v.Effective[k] = value
	}
	if err := p.Validate(); err != nil {
		v.Status = "mismatch"
		v.Reason = err.Error()
		return v
	}
	type check struct {
		key, want string
		cpus      bool
	}
	checks := []check{}
	if p.MemoryMaxBytes > 0 {
		checks = append(checks, check{"memory.max", strconv.FormatUint(p.MemoryMaxBytes, 10), false}, check{"memory.oom.group", "1", false})
	}
	if p.DisableSwap {
		checks = append(checks, check{"memory.swap.max", "0", false})
	}
	if p.CPUQuotaUS > 0 {
		checks = append(checks, check{"cpu.max", fmt.Sprintf("%d 100000", p.CPUQuotaUS), false})
	}
	if p.PidsMax > 0 {
		checks = append(checks, check{"pids.max", strconv.FormatUint(p.PidsMax, 10), false})
	}
	if p.CPUs != "" {
		checks = append(checks, check{"cpuset.cpus", p.CPUs, true}, check{"cpuset.cpus.effective", p.CPUs, true})
	}
	if p.Mems != "" {
		checks = append(checks, check{"cpuset.mems", p.Mems, true}, check{"cpuset.mems.effective", p.Mems, true})
	}
	if len(checks) == 0 {
		return v
	}
	v.Status = "verified"
	for _, c := range checks {
		value, ok := effective[c.key]
		if !ok || strings.HasPrefix(value, "unavailable:") || strings.TrimSpace(value) == "" {
			v.Status = "unavailable"
			v.Reason = "requested " + c.key + " readback unavailable"
			return v
		}
		matches := strings.Join(strings.Fields(value), " ") == c.want
		if c.cpus {
			want, _ := collectors.ParseCPUList(c.want)
			actual, err := collectors.ParseCPUList(value)
			matches = err == nil && slices.Equal(want, actual)
		}
		if !matches {
			v.Status = "mismatch"
			v.Reason = fmt.Sprintf("requested %s=%q, observed %q", c.key, c.want, value)
			return v
		}
	}
	return v
}

func (v ResourceVerification) Err() error {
	if v.Status == "verified" || v.Status == "not_requested" {
		return nil
	}
	return fmt.Errorf("resource policy %s at %s: %s", v.Status, v.Stage, v.Reason)
}

// ValidateNUMAIsolation validates successful evidence for the new explicit NUMA
// contract. Legacy receipts without a NUMA request are not retroactively upgraded.
func ValidateNUMAIsolation(p ResourcePolicy, iso *Isolation, finalStage string) error {
	if p.Mems == "" {
		return nil
	}
	if iso == nil || iso.Mode != "cgroup_v2_at_spawn" || filepath.Dir(iso.Path) != filepath.Clean(p.CgroupParent) {
		return fmt.Errorf("NUMA policy requires matching spawn isolation")
	}
	for _, pair := range []struct {
		v     *ResourceVerification
		stage string
	}{{iso.Verification, "before_spawn"}, {iso.FinalVerification, finalStage}} {
		if pair.v == nil {
			return fmt.Errorf("NUMA policy requires both boundary readbacks")
		}
		expected := CheckResourceReadback(p, pair.v.Effective, pair.stage)
		if expected.Status != "verified" || !reflect.DeepEqual(expected, *pair.v) {
			return fmt.Errorf("invalid or mismatched NUMA resource verification at %s", pair.stage)
		}
	}
	if !reflect.DeepEqual(iso.Effective, iso.Verification.Effective) {
		return fmt.Errorf("initial NUMA isolation evidence disagrees")
	}
	return nil
}
