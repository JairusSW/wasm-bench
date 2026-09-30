package sourcebuild

import (
	"fmt"
	"math"
	"os"
)

const CPUAccountingVersion = "go-process-state-cpu-v1"

// CPUAccounting is OS wait accounting, not cgroup-wide process-tree coverage.
// Descendant inclusion depends on OS accounting and child wait behavior.
type CPUAccounting struct {
	Collector string `json:"collector"`
	Scope     string `json:"scope"`
	Unit      string `json:"unit"`
	Accuracy  string `json:"accuracy"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	UserNS    *int64 `json:"user_ns"`
	SystemNS  *int64 `json:"system_ns"`
	TotalNS   *int64 `json:"total_ns"`
	Reason    string `json:"reason,omitempty"`
}

func captureCPU(p *os.ProcessState) *CPUAccounting {
	c := &CPUAccounting{Collector: "go_os_process_state", Scope: "os_waited_tool_process_usage", Unit: "ns", Accuracy: "os_reported", Version: CPUAccountingVersion, Status: "unavailable", Reason: "process resource accounting unavailable"}
	if p == nil || p.SysUsage() == nil {
		return c
	}
	u, s := p.UserTime().Nanoseconds(), p.SystemTime().Nanoseconds()
	if u < 0 || s < 0 || u > math.MaxInt64-s {
		c.Reason = "invalid or overflowing process CPU accounting"
		return c
	}
	total := u + s
	c.Status, c.Reason = "available", ""
	c.UserNS, c.SystemNS, c.TotalNS = &u, &s, &total
	return c
}

func (c *CPUAccounting) validate() error {
	if c == nil || c.Version != CPUAccountingVersion || c.Collector != "go_os_process_state" || c.Scope != "os_waited_tool_process_usage" || c.Unit != "ns" || c.Accuracy != "os_reported" {
		return fmt.Errorf("missing or unknown process CPU accounting")
	}
	if c.Status == "unavailable" {
		if c.UserNS != nil || c.SystemNS != nil || c.TotalNS != nil || c.Reason == "" {
			return fmt.Errorf("invalid unavailable CPU accounting")
		}
		return nil
	}
	if c.Status != "available" || c.UserNS == nil || c.SystemNS == nil || c.TotalNS == nil || *c.UserNS < 0 || *c.SystemNS < 0 || *c.UserNS > math.MaxInt64-*c.SystemNS || *c.TotalNS != *c.UserNS+*c.SystemNS || c.Reason != "" {
		return fmt.Errorf("invalid process CPU accounting")
	}
	return nil
}

func summedCPU(steps []StepResult) *int64 {
	var total int64
	if len(steps) == 0 {
		return nil
	}
	for _, s := range steps {
		if s.CPU.validate() != nil || s.CPU.Status != "available" || total > math.MaxInt64-*s.CPU.TotalNS {
			return nil
		}
		total += *s.CPU.TotalNS
	}
	return &total
}
