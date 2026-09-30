package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/collectors"
)

const IRQAffinityVersion = "read-only-irq-affinity-readiness-v1"
const IRQAffinityScope = "controller_namespace_default_requested_and_effective_device_irq_cpu_masks_at_two_boundaries_not_interrupt_delivery_or_continuous_machine_qualification"

type IRQAffinityProbe struct {
	Version string              `json:"version"`
	Scope   string              `json:"scope"`
	At      time.Time           `json:"at"`
	CPUs    string              `json:"measurement_cpus"`
	Status  string              `json:"status"`
	Reason  string              `json:"reason"`
	Facts   map[string]HostFact `json:"facts"`
}

func ValidateIRQAffinityPolicy(resources ResourcePolicy) error {
	if err := resources.Validate(); err != nil {
		return err
	}
	if _, err := collectors.ParseCPUList(resources.CPUs); err != nil {
		return fmt.Errorf("IRQ affinity requires explicit valid measurement CPUs: %w", err)
	}
	if resources.CgroupParent == "" {
		return fmt.Errorf("IRQ affinity requires a cgroup resource CPU budget")
	}
	return nil
}

func (p IRQAffinityProbe) Err() error {
	if err := ValidateIRQAffinityProbe(p); err != nil {
		return err
	}
	if p.Status != "disjoint_at_observed_boundaries" {
		return fmt.Errorf("IRQ affinity %s: %s", p.Status, p.Reason)
	}
	return nil
}

func ProbeIRQAffinity(cpus string) IRQAffinityProbe {
	p := IRQAffinityProbe{Version: IRQAffinityVersion, Scope: IRQAffinityScope, At: time.Now().UTC(), CPUs: cpus, Facts: map[string]HostFact{}}
	if cpus == "" {
		p.Status, p.Reason = "not_requested", "provide a complete measurement CPU list"
		return p
	}
	if _, err := collectors.ParseCPUList(cpus); err != nil {
		p.Status, p.Reason = "invalid_request", err.Error()
		return p
	}
	return probeIRQAffinityLive(p)
}

func readIRQAffinity(root fs.FS, p IRQAffinityProbe) IRQAffinityProbe {
	remaining := 8 << 20
	read := func(name string) HostFact {
		if remaining <= 0 {
			return HostFact{Source: "/" + name, Status: "too_large", Reason: "IRQ probe exceeds 8 MiB read budget"}
		}
		f, err := root.Open(name)
		if err != nil {
			return factError("/"+name, err)
		}
		defer f.Close()
		limit := min(64<<10, remaining)
		b, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
		remaining -= len(b)
		if err != nil {
			return factError("/"+name, err)
		}
		if len(b) > limit {
			return HostFact{Source: "/" + name, Status: "too_large", Reason: "IRQ source exceeds read budget"}
		}
		value := strings.TrimSpace(string(b))
		return HostFact{Source: "/" + name, Status: "available", Value: &value}
	}
	for _, stage := range []string{"before", "after"} {
		p.Facts[stage+"/online"] = read("sys/devices/system/cpu/online")
		p.Facts[stage+"/default"] = read("proc/irq/default_smp_affinity")
		f, err := root.Open("proc/irq")
		if err != nil {
			p.Facts[stage+"/inventory"] = factError("/proc/irq", err)
			continue
		}
		rd, ok := f.(fs.ReadDirFile)
		if !ok {
			f.Close()
			p.Facts[stage+"/inventory"] = HostFact{Source: "/proc/irq", Status: "read_error", Reason: "IRQ directory unavailable"}
			continue
		}
		entries, err := rd.ReadDir(4097)
		f.Close()
		if (err != nil && err != io.EOF) || len(entries) > 4096 {
			p.Facts[stage+"/inventory"] = HostFact{Source: "/proc/irq", Status: "unavailable", Reason: "IRQ inventory unavailable or exceeds 4096 directory entries"}
			continue
		}
		ids := []string{}
		valid := true
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			n, err := strconv.Atoi(entry.Name())
			if err != nil || n < 0 || strconv.Itoa(n) != entry.Name() {
				valid = false
				break
			}
			ids = append(ids, entry.Name())
		}
		if !valid {
			p.Facts[stage+"/inventory"] = HostFact{Source: "/proc/irq", Status: "read_error", Reason: "invalid IRQ directory inventory"}
			continue
		}
		slices.Sort(ids)
		encoded, _ := json.Marshal(ids)
		value := string(encoded)
		p.Facts[stage+"/inventory"] = HostFact{Source: "/proc/irq", Status: "available", Value: &value}
		for _, id := range ids {
			p.Facts[stage+"/irq/"+id+"/requested"] = read("proc/irq/" + id + "/smp_affinity_list")
			p.Facts[stage+"/irq/"+id+"/effective"] = read("proc/irq/" + id + "/effective_affinity_list")
		}
	}
	p.Status, p.Reason = CheckIRQAffinity(p)
	return p
}

// Kernel hexadecimal affinity masks consist of high-to-low 32-bit groups.
func parseIRQMask(value string) ([]int, error) {
	groups := strings.Split(value, ",")
	if len(groups) == 0 || len(groups) > 128 {
		return nil, fmt.Errorf("IRQ affinity mask exceeds 4096 CPUs")
	}
	cpus := []int{}
	for i, group := range groups {
		if len(group) == 0 || len(group) > 8 {
			return nil, fmt.Errorf("invalid IRQ mask group")
		}
		for _, r := range group {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return nil, fmt.Errorf("invalid IRQ hexadecimal mask")
			}
		}
		bits, err := strconv.ParseUint(group, 16, 32)
		if err != nil {
			return nil, err
		}
		for bit := 0; bit < 32; bit++ {
			if bits&(uint64(1)<<bit) != 0 {
				cpus = append(cpus, (len(groups)-i-1)*32+bit)
			}
		}
	}
	if len(cpus) == 0 {
		return nil, fmt.Errorf("empty IRQ affinity mask")
	}
	slices.Sort(cpus)
	return cpus, nil
}

func CheckIRQAffinity(p IRQAffinityProbe) (string, string) {
	if p.Version != IRQAffinityVersion || p.Scope != IRQAffinityScope || p.At.IsZero() {
		return "invalid_evidence", "unsupported IRQ affinity evidence"
	}
	if p.CPUs == "" {
		return "not_requested", "provide a complete measurement CPU list"
	}
	measurement, err := collectors.ParseCPUList(p.CPUs)
	if err != nil {
		return "invalid_request", err.Error()
	}
	raw := func(key string) (string, error) {
		f, ok := p.Facts[key]
		if !ok || f.Status != "available" || f.Value == nil {
			return "", fmt.Errorf("%s unavailable", key)
		}
		return *f.Value, nil
	}
	var firstOnline []int
	var firstIDs []string
	for _, stage := range []string{"before", "after"} {
		value, err := raw(stage + "/online")
		if err != nil {
			return "unavailable", err.Error()
		}
		online, err := collectors.ParseCPUList(value)
		if err != nil {
			return "invalid_evidence", "invalid online CPU list"
		}
		if stage == "after" && !slices.Equal(online, firstOnline) {
			return "not_ready", "online CPUs changed during IRQ probe"
		}
		firstOnline = online
		for _, cpu := range measurement {
			if !slices.Contains(online, cpu) {
				return "not_ready", "measurement CPU is offline"
			}
		}
		value, err = raw(stage + "/default")
		if err != nil {
			return "unavailable", err.Error()
		}
		mask, err := parseIRQMask(value)
		if err != nil {
			return "invalid_evidence", err.Error()
		}
		for _, cpu := range measurement {
			if slices.Contains(mask, cpu) {
				return "not_ready", "default IRQ affinity includes measurement CPUs"
			}
		}
		value, err = raw(stage + "/inventory")
		if err != nil {
			return "unavailable", err.Error()
		}
		var ids []string
		if json.Unmarshal([]byte(value), &ids) != nil || ids == nil || len(ids) > 4096 || !slices.IsSorted(ids) {
			return "invalid_evidence", "invalid IRQ inventory"
		}
		if stage == "after" && !slices.Equal(ids, firstIDs) {
			return "not_ready", "IRQ inventory changed during probe"
		}
		firstIDs = ids
		seen := map[string]bool{}
		for _, id := range ids {
			n, err := strconv.Atoi(id)
			if err != nil || n < 0 || strconv.Itoa(n) != id || seen[id] {
				return "invalid_evidence", "invalid IRQ inventory"
			}
			seen[id] = true
			for _, kind := range []string{"requested", "effective"} {
				key := stage + "/irq/" + id + "/" + kind
				value, err := raw(key)
				if err != nil {
					return "unavailable", err.Error()
				}
				mask, err := collectors.ParseCPUList(value)
				if err != nil {
					return "invalid_evidence", key + ": " + err.Error()
				}
				for _, cpu := range measurement {
					if slices.Contains(mask, cpu) {
						return "not_ready", key + " includes measurement CPUs"
					}
				}
			}
		}
	}
	return "disjoint_at_observed_boundaries", "default, requested and effective device IRQ masks exclude measurement CPUs at two read-only boundaries; delivery, local interrupts, unsampled intervals and dedicated-host control remain unqualified"
}

func ValidateIRQAffinityProbe(p IRQAffinityProbe) error {
	if p.Version != IRQAffinityVersion || p.Scope != IRQAffinityScope || p.At.IsZero() {
		return fmt.Errorf("unsupported IRQ affinity observation")
	}
	if len(p.Facts) == 0 && p.Status == "unsupported" && p.Reason == "device IRQ affinity probes require Linux procfs" {
		if _, err := collectors.ParseCPUList(p.CPUs); err != nil {
			return fmt.Errorf("invalid unsupported IRQ request: %w", err)
		}
		return nil
	}
	status, reason := CheckIRQAffinity(p)
	if p.Status != status || p.Reason != reason {
		return fmt.Errorf("IRQ affinity verdict disagrees with raw facts")
	}
	return nil
}
