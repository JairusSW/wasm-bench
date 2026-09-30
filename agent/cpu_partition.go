package agent

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/wasmbench/wasmbench/collectors"
)

const CPUPartitionVersion = "empty-isolated-partition-readiness-v1"
const CPUPartitionScope = "read_only_empty_partition_and_controller_threads_at_two_observed_boundaries_not_continuous_isolation_or_host_qualification"

type CPUPartitionProbe struct {
	Version string              `json:"version"`
	Path    string              `json:"path"`
	CPUs    string              `json:"requested_cpus"`
	Status  string              `json:"status"`
	Reason  string              `json:"reason"`
	Scope   string              `json:"scope"`
	Facts   map[string]HostFact `json:"facts"`
}

func newCPUPartitionProbe(partition, cpus string) CPUPartitionProbe {
	return CPUPartitionProbe{Version: CPUPartitionVersion, Path: partition, CPUs: cpus, Status: "not_requested", Reason: "provide an existing partition and its complete measurement CPU list", Scope: CPUPartitionScope, Facts: map[string]HostFact{}}
}

func ValidateCPUPartitionPolicy(resources ResourcePolicy) error {
	if err := resources.Validate(); err != nil {
		return err
	}
	return validatePartitionRequest(resources.CgroupParent, resources.CPUs)
}

func (p CPUPartitionProbe) Err() error {
	if err := ValidateCPUPartitionProbe(p); err != nil {
		return err
	}
	if p.Status == "ready_at_observed_boundaries" {
		return nil
	}
	return fmt.Errorf("CPU partition %s: %s", p.Status, p.Reason)
}

// Unreadable live filesystem receipts can only describe failure. Success must
// always be independently reproducible from complete raw boundary facts.
func ValidateCPUPartitionProbe(p CPUPartitionProbe) error {
	if p.Version != CPUPartitionVersion || p.Scope != CPUPartitionScope {
		return fmt.Errorf("unsupported CPU partition observation")
	}
	if err := validatePartitionRequest(p.Path, p.CPUs); err != nil {
		return err
	}
	if len(p.Facts) == 0 {
		if p.Reason != "" && (p.Status == "unavailable" || p.Status == "invalid_request" || p.Status == "unsupported") {
			return nil
		}
		return fmt.Errorf("CPU partition observation lacks boundary facts")
	}
	status, reason := CheckCPUPartition(p)
	if p.Status != status || p.Reason != reason {
		return fmt.Errorf("CPU partition status disagrees with raw boundary facts")
	}
	return nil
}

func validatePartitionRequest(partition, cpus string) error {
	if !filepath.IsAbs(partition) || filepath.Clean(partition) != partition || partition == "/" {
		return fmt.Errorf("partition must be a clean absolute non-root directory")
	}
	if _, err := collectors.ParseCPUList(cpus); err != nil {
		return fmt.Errorf("complete measurement CPU list required: %w", err)
	}
	return nil
}

// ProbeCPUPartition observes an existing partition; it never writes cgroup or
// affinity settings. Linux live probes also verify the cgroup v2 filesystem.
func ProbeCPUPartition(partition, cpus string) CPUPartitionProbe {
	p := newCPUPartitionProbe(partition, cpus)
	if partition == "" && cpus == "" {
		return p
	}
	if err := validatePartitionRequest(partition, cpus); err != nil {
		p.Status = "invalid_request"
		p.Reason = err.Error()
		return p
	}
	return probeCPUPartitionLive(p)
}

// The injected filesystem path is used by deterministic read-only tests.
func readCPUPartition(root fs.FS, p CPUPartitionProbe) CPUPartitionProbe {
	remaining := 8 << 20
	read := func(file string) HostFact {
		source := "/" + file
		f, err := root.Open(file)
		if err != nil {
			return factError(source, err)
		}
		defer f.Close()
		limit := min(64<<10, remaining)
		if limit <= 0 {
			return HostFact{Source: source, Status: "too_large", Reason: "probe exceeds 8 MiB read budget"}
		}
		b, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
		remaining -= len(b)
		if err != nil {
			return factError(source, err)
		}
		if len(b) > limit {
			return HostFact{Source: source, Status: "too_large", Reason: "source exceeds probe read budget"}
		}
		value := strings.TrimSpace(string(b))
		return HostFact{Source: source, Status: "available", Value: &value}
	}
	threadInventory := func() (HostFact, []string) {
		const dir = "proc/self/task"
		f, err := root.Open(dir)
		if err != nil {
			return factError("/"+dir, err), nil
		}
		defer f.Close()
		rd, ok := f.(fs.ReadDirFile)
		if !ok {
			return HostFact{Source: "/" + dir, Status: "read_error", Reason: "thread directory unavailable"}, nil
		}
		entries, err := rd.ReadDir(4097)
		if err != nil && err != io.EOF {
			return factError("/"+dir, err), nil
		}
		if len(entries) == 0 || len(entries) > 4096 {
			return HostFact{Source: "/" + dir, Status: "unavailable", Reason: "thread inventory empty or exceeds 4096 limit"}, nil
		}
		ids := []string{}
		for _, entry := range entries {
			n, e := strconv.Atoi(entry.Name())
			if e != nil || n <= 0 || !entry.IsDir() {
				return HostFact{Source: "/" + dir, Status: "read_error", Reason: "invalid thread inventory"}, nil
			}
			ids = append(ids, entry.Name())
		}
		slices.Sort(ids)
		value := strings.Join(ids, ",")
		return HostFact{Source: "/" + dir, Status: "available", Value: &value}, ids
	}
	partition := strings.TrimPrefix(p.Path, "/")
	for _, stage := range []string{"before", "after"} {
		for _, name := range []string{"cpuset.cpus.partition", "cpuset.cpus.effective", "cpuset.cpus.exclusive.effective", "cgroup.events"} {
			p.Facts[stage+"/"+name] = read(path.Join(partition, name))
		}
		p.Facts[stage+"/online"] = read("sys/devices/system/cpu/online")
		inventory, ids := threadInventory()
		p.Facts[stage+"/controller_threads"] = inventory
		for _, id := range ids {
			status := read("proc/self/task/" + id + "/status")
			p.Facts[stage+"/thread/"+id] = selectedHostFields(status, []string{"Cpus_allowed_list"})["Cpus_allowed_list"]
		}
		if stage == "before" {
			cpus, _ := collectors.ParseCPUList(p.CPUs)
			for _, cpu := range cpus {
				p.Facts["siblings/"+strconv.Itoa(cpu)] = read(fmt.Sprintf("sys/devices/system/cpu/cpu%d/topology/thread_siblings_list", cpu))
			}
		}
	}
	p.Status, p.Reason = CheckCPUPartition(p)
	return p
}

// CheckCPUPartition re-derives readiness from raw facts, never the saved status.
// Facts cannot authenticate their producer or prove continuous absence of work.
func CheckCPUPartition(p CPUPartitionProbe) (string, string) {
	if p.Version != CPUPartitionVersion {
		return "invalid_evidence", "unsupported partition evidence version"
	}
	if err := validatePartitionRequest(p.Path, p.CPUs); err != nil {
		return "invalid_evidence", err.Error()
	}
	expected, _ := collectors.ParseCPUList(p.CPUs)
	expectedSet := map[int]bool{}
	for _, cpu := range expected {
		expectedSet[cpu] = true
	}
	raw := func(key string) (string, error) {
		f, ok := p.Facts[key]
		if !ok || f.Status != "available" || f.Value == nil {
			return "", fmt.Errorf("%s unavailable (%s)", key, f.Status)
		}
		return *f.Value, nil
	}
	list := func(key string) ([]int, error) {
		v, err := raw(key)
		if err != nil {
			return nil, err
		}
		cpus, err := collectors.ParseCPUList(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		return cpus, nil
	}
	var onlineBefore []int
	var onlineSet map[int]bool
	var threadsBefore string
	for _, stage := range []string{"before", "after"} {
		mode, err := raw(stage + "/cpuset.cpus.partition")
		if err != nil {
			return "unavailable", err.Error()
		}
		if mode != "isolated" {
			return "not_ready", stage + ": partition is not a valid isolated root"
		}
		for _, key := range []string{"cpuset.cpus.effective", "cpuset.cpus.exclusive.effective"} {
			actual, err := list(stage + "/" + key)
			if err != nil {
				return "unavailable", err.Error()
			}
			if !slices.Equal(actual, expected) {
				return "not_ready", stage + ": " + key + " differs from complete requested CPU set"
			}
		}
		events, err := raw(stage + "/cgroup.events")
		if err != nil {
			return "unavailable", err.Error()
		}
		populated := ""
		count := 0
		for _, line := range strings.Split(events, "\n") {
			fields := strings.Fields(line)
			if len(fields) > 0 && fields[0] == "populated" {
				count++
				if len(fields) == 2 {
					populated = fields[1]
				}
			}
		}
		if count != 1 || populated != "0" {
			return "not_ready", stage + ": partition must report populated 0 before worker launch"
		}
		online, err := list(stage + "/online")
		if err != nil {
			return "unavailable", err.Error()
		}
		for _, cpu := range expected {
			if !slices.Contains(online, cpu) {
				return "not_ready", stage + ": requested CPU is offline"
			}
		}
		threads, err := raw(stage + "/controller_threads")
		if err != nil {
			return "unavailable", err.Error()
		}
		if threads == "" {
			return "unavailable", "empty controller thread inventory"
		}
		seen := map[string]bool{}
		for _, id := range strings.Split(threads, ",") {
			n, e := strconv.Atoi(id)
			if e != nil || n <= 0 || seen[id] || len(seen) >= 4096 {
				return "invalid_evidence", "invalid controller thread inventory"
			}
			seen[id] = true
			allowed, err := list(stage + "/thread/" + id)
			if err != nil {
				return "unavailable", err.Error()
			}
			for _, cpu := range allowed {
				if expectedSet[cpu] {
					return "not_ready", stage + ": controller thread " + id + " can run on measurement CPUs"
				}
			}
		}
		if stage == "before" {
			onlineBefore = online
			onlineSet = map[int]bool{}
			for _, cpu := range online {
				onlineSet[cpu] = true
			}
			threadsBefore = threads
		} else if !slices.Equal(onlineBefore, online) || threadsBefore != threads {
			return "changed_during_probe", "online CPUs or controller thread inventory changed between observations"
		}
	}
	for _, cpu := range expected {
		siblings, err := list("siblings/" + strconv.Itoa(cpu))
		if err != nil {
			return "unavailable", err.Error()
		}
		if !slices.Contains(siblings, cpu) {
			return "invalid_evidence", "CPU topology omits its own logical CPU"
		}
		for _, sibling := range siblings {
			if onlineSet[sibling] && !expectedSet[sibling] {
				return "not_ready", "online SMT sibling lies outside the isolated measurement partition"
			}
		}
	}
	return "ready_at_observed_boundaries", "empty isolated partition, complete online CPU/SMT coverage and disjoint controller thread masks observed; not continuous isolation, interrupt exclusion or dedicated-host qualification"
}
