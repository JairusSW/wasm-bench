package collectors

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// ParseCPUList accepts the kernel's comma/range cpulist representation, with
// bounded expansion. Duplicates, overlaps and malformed ranges are rejected.
func ParseCPUList(raw string) ([]int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 65536 {
		return nil, fmt.Errorf("empty or oversized CPU list")
	}
	seen := map[int]bool{}
	var cpus []int
	for _, part := range strings.Split(raw, ",") {
		bounds := strings.Split(part, "-")
		if len(bounds) > 2 {
			return nil, fmt.Errorf("invalid CPU range")
		}
		values := make([]int, len(bounds))
		for i, s := range bounds {
			if s == "" {
				return nil, fmt.Errorf("empty CPU index")
			}
			for _, c := range s {
				if c < '0' || c > '9' {
					return nil, fmt.Errorf("invalid CPU index")
				}
			}
			v, err := strconv.ParseUint(s, 10, 31)
			if err != nil {
				return nil, fmt.Errorf("invalid CPU index: %w", err)
			}
			values[i] = int(v)
		}
		start, end := values[0], values[0]
		if len(values) == 2 {
			end = values[1]
		}
		if end < start || end-start >= 4096 || len(cpus)+end-start+1 > 4096 {
			return nil, fmt.Errorf("reversed or oversized CPU range")
		}
		for offset := 0; offset <= end-start; offset++ {
			cpu := start + offset
			if seen[cpu] {
				return nil, fmt.Errorf("duplicate CPU %d", cpu)
			}
			seen[cpu] = true
			cpus = append(cpus, cpu)
		}
	}
	sort.Ints(cpus)
	return cpus, nil
}

func perfCoveredCPUs(effective, online string) ([]int, error) {
	e, err := ParseCPUList(effective)
	if err != nil {
		return nil, fmt.Errorf("effective cpuset: %w", err)
	}
	o, err := ParseCPUList(online)
	if err != nil {
		return nil, fmt.Errorf("online CPUs: %w", err)
	}
	var covered []int
	for _, cpu := range e {
		if slices.Contains(o, cpu) {
			covered = append(covered, cpu)
		}
	}
	if len(covered) == 0 {
		return nil, fmt.Errorf("no online CPUs in effective cpuset")
	}
	return covered, nil
}

func validatePerfCPUs(selected []int, effective, online string) error {
	covered, err := perfCoveredCPUs(effective, online)
	if err != nil {
		return err
	}
	s := append([]int(nil), selected...)
	sort.Ints(s)
	if !slices.Equal(s, covered) {
		return fmt.Errorf("perf CPUs must exactly cover online effective cpuset: selected %v, required %v", s, covered)
	}
	return nil
}
