package agent

import (
	"encoding/json"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func hostFixture() fstest.MapFS {
	files := map[string]string{
		"proc/cpuinfo":                                 "processor : 0\nmodel name : Test CPU\nflags : sse avx\nmicrocode : 0x12\ncpu MHz : 2000.00\nbogomips : 4000\n\nprocessor : 1\nmodel name : Other CPU\nflags : sse\nmicrocode : 0x13\n",
		"proc/self/status":                             "Pid: 100\nCpus_allowed_list: 0-1\nMems_allowed_list: 0\nVmRSS: 99 kB\n",
		"proc/meminfo":                                 "MemTotal: 1000000 kB\nMemFree: 900000 kB\nSwapTotal: 100 kB\nHugePages_Total: 2\nHugePages_Free: 1\nHugepagesize: 2048 kB\n",
		"sys/devices/system/cpu/online":                "0-1\n",
		"sys/devices/system/cpu/offline":               "\n",
		"sys/devices/system/cpu/cpu0/topology/core_id": "0\n",
		"sys/devices/system/cpu/cpu0/topology/thread_siblings_list": "0-1\n",
		"sys/devices/system/cpu/cpu1/topology/core_id":              "0\n",
		"sys/devices/system/cpu/cpufreq/policy0/scaling_governor":   "performance\n",
		"sys/devices/system/cpu/cpufreq/policy0/scaling_min_freq":   "1000000\n",
		"sys/devices/system/cpu/cpufreq/policy0/scaling_cur_freq":   "2000000\n",
		"sys/devices/system/cpu/smt/control":                        "on\n",
		"sys/devices/system/node/node0/cpulist":                     "0-1\n",
		"sys/kernel/mm/transparent_hugepage/enabled":                "always [madvise] never\n",
		"sys/kernel/mm/transparent_hugepage/hugepages-64kB/enabled": "always inherit [madvise] never\n",
		"sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages":     "2\n",
	}
	out := fstest.MapFS{}
	for path, value := range files {
		out[path] = &fstest.MapFile{Data: []byte(value)}
	}
	return out
}
func TestLinuxHostFacts(t *testing.T) {
	fingerprint, model := linuxHostFacts(hostFixture())
	if model != "Other CPU; Test CPU" || fingerprint.Version != HostFingerprintVersion {
		t.Fatal(model, fingerprint.Version)
	}
	for key, want := range map[string]string{
		"proc/cpuinfo/record-0/flags": "avx sse", "proc/cpuinfo/record-1/flags": "sse", "proc/cpuinfo/record-1/microcode": "0x13",
		"sys/devices/system/cpu/inventory": "cpu0,cpu1", "sys/devices/system/cpu/offline": "", "proc/self/status#Cpus_allowed_list": "0-1",
		"sys/devices/system/cpu/cpufreq/policy0/scaling_governor": "performance", "sys/devices/system/node/node0/cpulist": "0-1",
		"sys/kernel/mm/transparent_hugepage/hugepages-64kB/enabled": "always inherit [madvise] never",
		"sys/kernel/mm/hugepages/hugepages-2048kB/nr_hugepages":     "2",
	} {
		fact := fingerprint.Facts[key]
		if fact.Status != "available" || fact.Value == nil || *fact.Value != want || fact.Source == "" {
			t.Fatal(key, fact)
		}
	}
	missing := fingerprint.Facts["sys/devices/system/cpu/cpu0/topology/die_id"]
	if missing.Status != "not_exposed" || missing.Value != nil {
		t.Fatal(missing)
	}
	bytes, _ := json.Marshal(fingerprint)
	for _, forbidden := range []string{"cpu MHz", "bogomips", "MemFree", "VmRSS", "HugePages_Free", "scaling_cur_freq"} {
		if strings.Contains(string(bytes), forbidden) {
			t.Fatal("transient data included", forbidden)
		}
	}
}
func TestHostFactsStableCountersSensitiveSettings(t *testing.T) {
	original, _ := linuxHostFacts(hostFixture())
	changed := hostFixture()
	changed["proc/cpuinfo"].Data = []byte(strings.ReplaceAll(strings.ReplaceAll(string(changed["proc/cpuinfo"].Data), "2000.00", "1000.00"), "flags : sse avx", "flags : avx sse"))
	changed["proc/self/status"].Data = []byte(strings.ReplaceAll(string(changed["proc/self/status"].Data), "Pid: 100", "Pid: 200"))
	changed["proc/meminfo"].Data = []byte(strings.ReplaceAll(string(changed["proc/meminfo"].Data), "900000", "1234"))
	next, _ := linuxHostFacts(changed)
	if !reflect.DeepEqual(original, next) {
		t.Fatal("transient counters changed fingerprint")
	}
	for _, path := range []string{"sys/devices/system/cpu/cpufreq/policy0/scaling_governor", "sys/devices/system/cpu/cpu0/topology/core_id", "sys/devices/system/cpu/smt/control", "sys/kernel/mm/transparent_hugepage/enabled", "proc/cpuinfo", "proc/self/status"} {
		changed := hostFixture()
		changed[path].Data = []byte("changed")
		next, _ := linuxHostFacts(changed)
		if reflect.DeepEqual(original, next) {
			t.Fatal("ignored changed configuration", path)
		}
	}
}

type deniedHostFS struct{ fs.FS }

func (f deniedHostFS) Open(path string) (fs.File, error) {
	if path == "proc/cpuinfo" || path == "sys/devices/system/cpu" {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(path)
}
func TestHostFactsUnavailableAndBounds(t *testing.T) {
	fingerprint, _ := linuxHostFacts(deniedHostFS{fstest.MapFS{}})
	for _, key := range []string{"proc/cpuinfo", "sys/devices/system/cpu/inventory"} {
		if f := fingerprint.Facts[key]; f.Status != "permission_denied" || f.Value != nil {
			t.Fatal(f)
		}
	}
	if f := fingerprint.Facts["proc/self/status#Cpus_allowed_list"]; f.Status != "not_exposed" || f.Value != nil {
		t.Fatal(f)
	}
	large := fstest.MapFS{"large": &fstest.MapFile{Data: []byte(strings.Repeat("x", hostFactLimit+1))}}
	if f := readHostFact(large, "large"); f.Status != "too_large" || f.Value != nil {
		t.Fatal(f)
	}
}
func TestARMHostFacts(t *testing.T) {
	fixture := fstest.MapFS{"proc/cpuinfo": &fstest.MapFile{Data: []byte("processor: 0\nFeatures: asimd fp\nCPU implementer: 0x41\nCPU part: 0xd0c\nCPU revision: 1\n")}}
	fingerprint, description := linuxHostFacts(fixture)
	if description != "not exposed; see fingerprint CPU identity fields" {
		t.Fatal(description)
	}
	if f := fingerprint.Facts["proc/cpuinfo/record-0/Features"]; f.Value == nil || *f.Value != "asimd fp" {
		t.Fatal(f)
	}
	if f := fingerprint.Facts["proc/cpuinfo/record-0/microcode"]; f.Status != "not_exposed" || f.Value != nil {
		t.Fatal(f)
	}
}

func TestLegacyHostFactsAreNotInvented(t *testing.T) {
	var host Host
	if err := json.Unmarshal([]byte(`{"os":"linux","cpu_description":"legacy raw cpuinfo"}`), &host); err != nil {
		t.Fatal(err)
	}
	if host.Fingerprint != nil {
		t.Fatal("backfilled legacy facts")
	}
	encoded, err := json.Marshal(host)
	if err != nil || strings.Contains(string(encoded), "fingerprint") {
		t.Fatal(string(encoded), err)
	}
}
