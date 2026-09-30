package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wasmbench/wasmbench/collectors"
)

const LegacyActivePartitionVersion = "sampled-occupied-partition-v1"
const LegacyActivePartitionScope = "read_only_sample_of_partition_masks_population_and_single_worker_membership_not_continuous_isolation_or_host_qualification"
const ActivePartitionVersion = "sampled-occupied-partition-v2"
const ActivePartitionScope = "read_only_sample_of_partition_masks_population_single_worker_and_controller_thread_affinity_not_continuous_isolation_or_host_qualification"

type ActivePartitionSample struct {
	Version string              `json:"version"`
	Scope   string              `json:"scope"`
	At      time.Time           `json:"at"`
	Parent  string              `json:"parent"`
	Worker  string              `json:"worker"`
	CPUs    string              `json:"requested_cpus"`
	PID     int                 `json:"adapter_pid"`
	Facts   map[string]HostFact `json:"facts"`
	Status  string              `json:"status"`
	Reason  string              `json:"reason"`
}

func activePartitionRequest(parent, worker, cpus string, pid int) error {
	if err := validatePartitionRequest(parent, cpus); err != nil {
		return err
	}
	if pid <= 0 || !filepath.IsAbs(worker) || filepath.Clean(worker) != worker || filepath.Dir(worker) != parent || !strings.HasPrefix(filepath.Base(worker), "wasmbench-") {
		return fmt.Errorf("worker must be a direct wasmbench cgroup child with an adapter PID")
	}
	return nil
}

func activeFact(root fs.FS, name string) HostFact {
	f, err := root.Open(name)
	if err != nil {
		return factError("/"+name, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil {
		return factError("/"+name, err)
	}
	if len(b) > 64<<10 {
		return HostFact{Source: "/" + name, Status: "too_large", Reason: "source exceeds 64 KiB read budget"}
	}
	value := strings.TrimSpace(string(b))
	return HostFact{Source: "/" + name, Status: "available", Value: &value}
}

// readActivePartition samples an already occupied partition without changing it.
// It is intentionally not a continuous or dedicated-host certification.
func readActivePartition(root fs.FS, parent, worker, cpus string, pid int, at time.Time) ActivePartitionSample {
	s := ActivePartitionSample{Version: ActivePartitionVersion, Scope: ActivePartitionScope, At: at, Parent: parent, Worker: worker, CPUs: cpus, PID: pid, Facts: map[string]HostFact{}}
	if err := activePartitionRequest(parent, worker, cpus, pid); err != nil {
		s.Status, s.Reason = "invalid_request", err.Error()
		return s
	}
	readActiveControllerThreads(root, s.Facts)
	p := strings.TrimPrefix(parent, "/")
	w := strings.TrimPrefix(worker, "/")
	for _, spec := range []struct{ key, name string }{
		{"partition", path.Join(p, "cpuset.cpus.partition")},
		{"effective_cpus", path.Join(p, "cpuset.cpus.effective")},
		{"exclusive_cpus", path.Join(p, "cpuset.cpus.exclusive.effective")},
		{"parent_events", path.Join(p, "cgroup.events")},
		{"parent_procs", path.Join(p, "cgroup.procs")},
		{"worker_events", path.Join(w, "cgroup.events")},
		{"worker_procs", path.Join(w, "cgroup.procs")},
		{"worker_cpus", path.Join(w, "cpuset.cpus.effective")},
		{"online", "sys/devices/system/cpu/online"},
	} {
		s.Facts[spec.key] = activeFact(root, spec.name)
	}
	dir, err := root.Open(p)
	if err != nil {
		s.Facts["children"] = factError("/"+p, err)
	} else {
		reader, ok := dir.(fs.ReadDirFile)
		if !ok {
			s.Facts["children"] = HostFact{Source: "/" + p, Status: "read_error", Reason: "child directory unavailable"}
		} else {
			entries, e := reader.ReadDir(4097)
			children := []string{}
			for _, entry := range entries {
				if entry.IsDir() {
					children = append(children, entry.Name())
					if len(children) <= 4096 {
						s.Facts["child/"+entry.Name()] = activeFact(root, path.Join(p, entry.Name(), "cgroup.events"))
					}
				}
			}
			if e != nil && e != io.EOF {
				s.Facts["children"] = factError("/"+p, e)
			} else if len(entries) > 4096 {
				s.Facts["children"] = HostFact{Source: "/" + p, Status: "too_large", Reason: "child cgroup inventory exceeds 4096 entries"}
			} else {
				slices.Sort(children)
				encoded, _ := json.Marshal(children)
				value := string(encoded)
				s.Facts["children"] = HostFact{Source: "/" + p, Status: "available", Value: &value}
			}
		}
		dir.Close()
	}
	s.Status, s.Reason = CheckActivePartition(s)
	return s
}

func populated(events string) (string, error) {
	var value string
	count := 0
	for _, line := range strings.Split(events, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "populated" {
			count++
			if len(fields) == 2 {
				value = fields[1]
			}
		}
	}
	if count != 1 || (value != "0" && value != "1") {
		return "", fmt.Errorf("invalid cgroup population receipt")
	}
	return value, nil
}

func CheckActivePartition(s ActivePartitionSample) (string, string) {
	legacy := s.Version == LegacyActivePartitionVersion && s.Scope == LegacyActivePartitionScope
	if (!legacy && (s.Version != ActivePartitionVersion || s.Scope != ActivePartitionScope)) || s.At.IsZero() {
		return "invalid_evidence", "unsupported active partition sample"
	}
	if err := activePartitionRequest(s.Parent, s.Worker, s.CPUs, s.PID); err != nil {
		return "invalid_evidence", err.Error()
	}
	raw := func(key string) (string, error) {
		f, ok := s.Facts[key]
		if !ok || f.Status != "available" || f.Value == nil {
			return "", fmt.Errorf("%s unavailable", key)
		}
		return *f.Value, nil
	}
	mode, err := raw("partition")
	if err != nil {
		return "unavailable", err.Error()
	}
	if mode != "isolated" {
		return "not_ready", "partition is not an isolated root"
	}
	expected, _ := collectors.ParseCPUList(s.CPUs)
	if !legacy {
		if status, reason := checkActiveControllerThreads(s.Facts, expected); status != "ready" {
			return status, reason
		}
	}
	for _, key := range []string{"effective_cpus", "exclusive_cpus", "worker_cpus"} {
		v, err := raw(key)
		if err != nil {
			return "unavailable", err.Error()
		}
		got, err := collectors.ParseCPUList(v)
		if err != nil {
			return "invalid_evidence", key + ": " + err.Error()
		}
		if !slices.Equal(got, expected) {
			return "not_ready", key + " differs from requested CPUs"
		}
	}
	online, err := raw("online")
	if err != nil {
		return "unavailable", err.Error()
	}
	onlineCPUs, err := collectors.ParseCPUList(online)
	if err != nil {
		return "invalid_evidence", "online CPUs: " + err.Error()
	}
	for _, cpu := range expected {
		if !slices.Contains(onlineCPUs, cpu) {
			return "not_ready", "requested CPU became offline"
		}
	}
	for _, key := range []string{"parent_events", "worker_events"} {
		v, err := raw(key)
		if err != nil {
			return "unavailable", err.Error()
		}
		p, err := populated(v)
		if err != nil {
			return "invalid_evidence", key + ": " + err.Error()
		}
		if p != "1" {
			return "not_ready", key + " did not report populated 1"
		}
	}
	rootProcs, err := raw("parent_procs")
	if err != nil {
		return "unavailable", err.Error()
	}
	if rootProcs != "" {
		return "not_ready", "process found in partition root outside worker cgroup"
	}
	workerProcs, err := raw("worker_procs")
	if err != nil {
		return "unavailable", err.Error()
	}
	if workerProcs != strconv.Itoa(s.PID) {
		return "not_ready", "worker cgroup does not contain exactly the adapter PID"
	}
	children, err := raw("children")
	if err != nil {
		return "unavailable", err.Error()
	}
	var childNames []string
	if err := json.Unmarshal([]byte(children), &childNames); err != nil || len(childNames) == 0 || len(childNames) > 4096 {
		return "invalid_evidence", "invalid child cgroup inventory"
	}
	workerName := filepath.Base(s.Worker)
	found := false
	seen := map[string]bool{}
	for _, child := range childNames {
		if child == "" || strings.ContainsAny(child, "/\\") || seen[child] {
			return "invalid_evidence", "invalid child cgroup inventory"
		}
		seen[child] = true
		if child == workerName {
			found = true
			continue
		}
		v, err := raw("child/" + child)
		if err != nil {
			return "unavailable", err.Error()
		}
		p, err := populated(v)
		if err != nil {
			return "invalid_evidence", "child/" + child + ": " + err.Error()
		}
		if p != "0" {
			return "not_ready", "another child cgroup is populated"
		}
	}
	if !found {
		return "invalid_evidence", "worker missing from child inventory"
	}
	return "ready_at_sample", "requested CPU masks and single adapter worker observed; sampled evidence cannot prove uninterrupted isolation or dedicated-host control"
}

func ValidateActivePartitionSample(s ActivePartitionSample) error {
	if len(s.Facts) == 0 && s.Reason != "" && (s.Status == "unavailable" || s.Status == "unsupported" || s.Status == "invalid_request") {
		return nil
	}
	status, reason := CheckActivePartition(s)
	if s.Status != status || s.Reason != reason {
		return fmt.Errorf("active partition status disagrees with raw facts")
	}
	return nil
}

type PartitionMonitor struct {
	Version    string                  `json:"version"`
	IntervalNS int64                   `json:"interval_ns"`
	Overflow   bool                    `json:"overflow,omitempty"`
	Status     string                  `json:"status"`
	Reason     string                  `json:"reason"`
	Samples    []ActivePartitionSample `json:"samples"`

	mu     sync.Mutex
	stop   chan struct{}
	done   chan struct{}
	parent string
	worker string
	cpus   string
	pid    int
}

const partitionSampleInterval = 250 * time.Millisecond
const maxPartitionSamples = 8192

func StartPartitionMonitor(parent, worker, cpus string, pid int) *PartitionMonitor {
	m := &PartitionMonitor{Version: ActivePartitionVersion, IntervalNS: partitionSampleInterval.Nanoseconds(), Samples: []ActivePartitionSample{}, stop: make(chan struct{}), done: make(chan struct{}), parent: parent, worker: worker, cpus: cpus, pid: pid}
	m.sample()
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(partitionSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-m.stop:
				return
			case <-ticker.C:
				m.sample()
			}
		}
	}()
	return m
}

func (m *PartitionMonitor) sample() {
	s := ProbeActiveCPUPartition(m.parent, m.worker, m.cpus, m.pid)
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Samples) < maxPartitionSamples {
		m.Samples = append(m.Samples, s)
	} else {
		m.Overflow = true
		m.Status, m.Reason = "overflow", "active partition sample limit exceeded"
	}
}

func (m *PartitionMonitor) Stop() *PartitionMonitor {
	close(m.stop)
	<-m.done
	m.sample()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.Overflow {
		m.Status, m.Reason = "ready_at_samples", "all recorded samples show the single adapter worker; unsampled intervals remain unqualified"
		for _, s := range m.Samples {
			if s.Status != "ready_at_sample" {
				m.Status, m.Reason = "not_ready", "one or more active partition samples were not ready"
				break
			}
		}
	}
	return m
}

// ValidatePartitionMonitor re-derives the sampled verdict from sealed facts.
// It cannot authenticate the observer or fill gaps between samples.
func ValidatePartitionMonitor(m *PartitionMonitor) error {
	if m == nil || (m.Version != ActivePartitionVersion && m.Version != LegacyActivePartitionVersion) || m.IntervalNS != partitionSampleInterval.Nanoseconds() || len(m.Samples) == 0 || len(m.Samples) > maxPartitionSamples {
		return fmt.Errorf("invalid active partition monitor contract")
	}
	first := m.Samples[0]
	ready := true
	for i, s := range m.Samples {
		if s.Version != m.Version {
			return fmt.Errorf("active partition monitor mixes evidence versions")
		}
		if err := ValidateActivePartitionSample(s); err != nil {
			return fmt.Errorf("partition sample %d: %w", i, err)
		}
		if s.Parent != first.Parent || s.Worker != first.Worker || s.CPUs != first.CPUs || s.PID != first.PID || (i > 0 && s.At.Before(m.Samples[i-1].At)) {
			return fmt.Errorf("active partition sample identity or order changed")
		}
		ready = ready && s.Status == "ready_at_sample"
	}
	if m.Overflow {
		if len(m.Samples) != maxPartitionSamples || m.Status != "overflow" || m.Reason != "active partition sample limit exceeded" {
			return fmt.Errorf("invalid active partition monitor overflow evidence")
		}
		return nil
	}
	status, reason := "ready_at_samples", "all recorded samples show the single adapter worker; unsampled intervals remain unqualified"
	if !ready {
		status, reason = "not_ready", "one or more active partition samples were not ready"
	}
	if m.Status != status || m.Reason != reason {
		return fmt.Errorf("active partition monitor verdict disagrees with samples")
	}
	return nil
}
