package agent

import (
	"fmt"
	"github.com/wasmbench/wasmbench/collectors"
	"path/filepath"
)

// ResourcePolicy is requested before process creation. No cgroup request ever
// falls back to an unisolated launch. An empty parent opts out of isolation.
type ResourcePolicy struct {
	CgroupParent   string `json:"cgroup_parent,omitempty"`
	MemoryMaxBytes uint64 `json:"memory_max_bytes,omitempty"`
	DisableSwap    bool   `json:"disable_swap,omitempty"`
	CPUQuotaUS     uint64 `json:"cpu_quota_us_per_100000,omitempty"`
	CPUs           string `json:"cpus,omitempty"`
	Mems           string `json:"mems,omitempty"`
	PidsMax        uint64 `json:"pids_max,omitempty"`
}

func (p ResourcePolicy) Validate() error {
	if p.CgroupParent == "" {
		if p.MemoryMaxBytes != 0 || p.DisableSwap || p.CPUQuotaUS != 0 || p.CPUs != "" || p.Mems != "" || p.PidsMax != 0 {
			return fmt.Errorf("resource limits require --cgroup-parent")
		}
		return nil
	}
	if !filepath.IsAbs(p.CgroupParent) {
		return fmt.Errorf("cgroup parent must be absolute")
	}
	if p.CPUs != "" {
		if _, err := collectors.ParseCPUList(p.CPUs); err != nil {
			return fmt.Errorf("invalid CPU list: %w", err)
		}
	}
	if p.CPUQuotaUS > 0 && p.CPUQuotaUS < 1000 {
		return fmt.Errorf("CPU quota must be at least 1000 microseconds per 100000")
	}
	if p.Mems != "" {
		if _, err := collectors.ParseCPUList(p.Mems); err != nil {
			return fmt.Errorf("invalid NUMA node list: %w", err)
		}
	}
	return nil
}

type Isolation struct {
	Verification      *ResourceVerification `json:"verification,omitempty"`
	FinalVerification *ResourceVerification `json:"final_verification,omitempty"`
	Mode              string                `json:"mode"`
	Path              string                `json:"path,omitempty"`
	Effective         map[string]string     `json:"effective,omitempty"`
}
