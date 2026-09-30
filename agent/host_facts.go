package agent

import (
	"errors"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

const HostFingerprintVersion = "linux-host-facts-v1"
const hostFactLimit = 8 << 20

// HostFact records a raw kernel setting, not proof that the runner controls it.
// Empty values can be valid (for example an empty offline CPU list); missing
// values use an explicit status and nil Value rather than an invented zero.
type HostFact struct {
	Source string  `json:"source"`
	Status string  `json:"status"`
	Value  *string `json:"value"`
	Reason string  `json:"reason,omitempty"`
}
type HostFingerprint struct {
	Version        string              `json:"version"`
	Scope          string              `json:"scope"`
	Interpretation string              `json:"interpretation"`
	Facts          map[string]HostFact `json:"facts"`
}

func factError(source string, err error) HostFact {
	status, reason := "read_error", "kernel source could not be read"
	if errors.Is(err, fs.ErrNotExist) {
		status, reason = "not_exposed", "source is absent in this namespace"
	} else if errors.Is(err, fs.ErrPermission) {
		status, reason = "permission_denied", "source exists but access is denied"
	}
	return HostFact{Source: source, Status: status, Reason: reason}
}
func readHostFact(root fs.FS, path string) HostFact {
	source := "/" + path
	f, err := root.Open(path)
	if err != nil {
		return factError(source, err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, hostFactLimit+1))
	if err != nil {
		return factError(source, err)
	}
	if len(b) > hostFactLimit {
		return HostFact{Source: source, Status: "too_large", Reason: "source exceeds 8 MiB bound"}
	}
	value := strings.TrimSpace(string(b))
	return HostFact{Source: source, Status: "available", Value: &value}
}
func selectedHostFields(source HostFact, wanted []string) map[string]HostFact {
	out := map[string]HostFact{}
	for _, key := range wanted {
		v := source
		v.Value = nil
		if source.Status == "available" {
			v.Status = "not_exposed"
			v.Reason = "field absent"
		}
		v.Source += "#" + key
		out[key] = v
	}
	if source.Value == nil {
		return out
	}
	for _, line := range strings.Split(*source.Value, "\n") {
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if _, wanted := out[key]; !ok || !wanted {
			continue
		}
		value = strings.Join(strings.Fields(value), " ")
		v := out[key]
		v.Status, v.Reason, v.Value = "available", "", &value
		out[key] = v
	}
	return out
}
func hostInventory(root fs.FS, path, prefix, suffix string) (HostFact, []string) {
	entries, err := fs.ReadDir(root, path)
	if err != nil {
		return factError("/"+path, err), nil
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) {
			continue
		}
		number := strings.TrimSuffix(strings.TrimPrefix(name, prefix), suffix)
		if number == "" {
			continue
		}
		if _, err := strconv.ParseUint(number, 10, 32); err == nil {
			names = append(names, name)
		}
	}
	if len(names) > 16384 {
		return HostFact{Source: "/" + path, Status: "too_large", Reason: "inventory exceeds 16384 entries"}, nil
	}
	sort.Strings(names)
	value := strings.Join(names, ",")
	return HostFact{Source: "/" + path, Status: "available", Value: &value}, names
}

// linuxHostFacts is filesystem-injected so unavailable attributes, heterogeneous
// CPU records, namespaces and changing transient counters can be tested on any OS.
func linuxHostFacts(root fs.FS) (*HostFingerprint, string) {
	result := &HostFingerprint{Version: HostFingerprintVersion, Scope: "controller_namespace_at_manifest_creation", Interpretation: "Read-only observed kernel settings, not enforced machine policy. CPU/NUMA lists describe allowed locations, not actual placement. Containers may hide host settings. No frequency, memory-pressure, affinity or isolation guarantee is inferred. Transient current frequency and free-memory counters are excluded.", Facts: map[string]HostFact{}}
	facts := result.Facts
	add := func(path string) { facts[path] = readHostFact(root, path) }
	for _, path := range []string{
		"sys/devices/system/cpu/online", "sys/devices/system/cpu/offline", "sys/devices/system/cpu/possible", "sys/devices/system/cpu/present", "sys/devices/system/cpu/isolated", "sys/devices/system/cpu/nohz_full",
		"sys/devices/system/cpu/smt/control", "sys/devices/system/cpu/smt/active", "sys/devices/system/cpu/cpufreq/boost", "sys/devices/system/cpu/intel_pstate/no_turbo",
		"sys/devices/system/node/online", "sys/devices/system/node/possible",
		"sys/kernel/mm/transparent_hugepage/enabled", "sys/kernel/mm/transparent_hugepage/defrag", "sys/kernel/mm/transparent_hugepage/shmem_enabled",
		"proc/sys/vm/nr_hugepages", "proc/sys/vm/overcommit_memory", "proc/sys/vm/overcommit_ratio", "proc/sys/vm/swappiness", "proc/sys/vm/max_map_count", "proc/sys/kernel/numa_balancing",
	} {
		add(path)
	}
	for _, spec := range []struct {
		path, prefix, suffix string
		attributes           []string
	}{
		{"sys/devices/system/cpu", "cpu", "", []string{"topology/physical_package_id", "topology/die_id", "topology/core_id", "topology/thread_siblings_list", "topology/core_siblings_list", "topology/cluster_id", "microcode/version"}},
		{"sys/devices/system/cpu/cpufreq", "policy", "", []string{"scaling_driver", "scaling_governor", "scaling_min_freq", "scaling_max_freq", "cpuinfo_min_freq", "cpuinfo_max_freq", "affected_cpus", "related_cpus", "energy_performance_preference"}},
		{"sys/devices/system/node", "node", "", []string{"cpulist", "distance"}},
		{"sys/kernel/mm/transparent_hugepage", "hugepages-", "kB", []string{"enabled", "shmem_enabled"}},
		{"sys/kernel/mm/hugepages", "hugepages-", "kB", []string{"nr_hugepages", "nr_overcommit_hugepages"}},
	} {
		inventory, names := hostInventory(root, spec.path, spec.prefix, spec.suffix)
		facts[spec.path+"/inventory"] = inventory
		for _, name := range names {
			for _, attribute := range spec.attributes {
				add(spec.path + "/" + name + "/" + attribute)
			}
		}
	}
	for _, spec := range []struct {
		path   string
		fields []string
	}{
		{"proc/self/status", []string{"Cpus_allowed_list", "Mems_allowed_list"}},
		{"proc/meminfo", []string{"MemTotal", "SwapTotal", "HugePages_Total", "Hugepagesize"}},
	} {
		for key, value := range selectedHostFields(readHostFact(root, spec.path), spec.fields) {
			facts[spec.path+"#"+key] = value
		}
	}
	// Do not store raw cpuinfo: MHz and BogoMIPS can vary without changing the
	// configuration. Preserve each CPU's identity/features rather than taking a
	// union that would conceal heterogeneous cores or missing microcode fields.
	source := readHostFact(root, "proc/cpuinfo")
	wanted := []string{"processor", "vendor_id", "cpu family", "model", "model name", "stepping", "microcode", "flags", "Features", "CPU implementer", "CPU architecture", "CPU variant", "CPU part", "CPU revision", "Hardware"}
	models := map[string]bool{}
	if source.Value == nil {
		facts["proc/cpuinfo"] = source
	} else {
		records := strings.Split(strings.ReplaceAll(*source.Value, "\r\n", "\n"), "\n\n")
		for i, record := range records {
			if strings.TrimSpace(record) == "" {
				continue
			}
			one := source
			one.Value = &record
			one.Source += "#record-" + strconv.Itoa(i)
			for key, value := range selectedHostFields(one, wanted) {
				if value.Value != nil && (key == "flags" || key == "Features") {
					tokens := strings.Fields(*value.Value)
					sort.Strings(tokens)
					normalized := strings.Join(tokens, " ")
					value.Value = &normalized
				}
				facts["proc/cpuinfo/record-"+strconv.Itoa(i)+"/"+key] = value
				if value.Value != nil && (key == "model name" || key == "Hardware") {
					models[*value.Value] = true
				}
			}
		}
	}
	var descriptions []string
	for model := range models {
		descriptions = append(descriptions, model)
	}
	sort.Strings(descriptions)
	description := strings.Join(descriptions, "; ")
	if description == "" {
		description = "not exposed; see fingerprint CPU identity fields"
	}
	return result, description
}
