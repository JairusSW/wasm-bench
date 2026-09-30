package agent

import (
	"encoding/json"
	"io/fs"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func partitionFixture() fstest.MapFS {
	m := fstest.MapFS{}
	for name, value := range map[string]string{
		"sys/fs/cgroup/measurement/cpuset.cpus.partition":           "isolated\n",
		"sys/fs/cgroup/measurement/cpuset.cpus.effective":           "2-3\n",
		"sys/fs/cgroup/measurement/cpuset.cpus.exclusive.effective": "2,3\n",
		"sys/fs/cgroup/measurement/cgroup.events":                   "populated 0\nfrozen 0\n",
		"sys/devices/system/cpu/online":                             "0-3\n",
		"sys/devices/system/cpu/cpu2/topology/thread_siblings_list": "2-3\n",
		"sys/devices/system/cpu/cpu3/topology/thread_siblings_list": "2,3\n",
		"proc/self/task/10/status":                                  "Name:\tcontroller\nCpus_allowed_list:\t0-1\n",
		"proc/self/task/11/status":                                  "Name:\tcollector\nCpus_allowed_list:\t1\n",
	} {
		m[name] = &fstest.MapFile{Data: []byte(value)}
	}
	return m
}

func TestCPUPartitionReadOnlyReadiness(t *testing.T) {
	root := partitionFixture()
	before, _ := json.Marshal(root)
	p := readCPUPartition(root, newCPUPartitionProbe("/sys/fs/cgroup/measurement", "2-3"))
	if p.Status != "ready_at_observed_boundaries" {
		t.Fatal(p.Status, p.Reason)
	}
	after, _ := json.Marshal(root)
	if string(before) != string(after) {
		t.Fatal("probe mutated sources")
	}
	if len(p.Facts) != 18 {
		t.Fatal("missing boundary facts", len(p.Facts))
	}
	if p.Facts["before/thread/11"].Value == nil || *p.Facts["before/thread/11"].Value != "1" {
		t.Fatal("collector thread not observed")
	}
	p.Status = "forged-status"
	status, _ := CheckCPUPartition(p)
	if status != "ready_at_observed_boundaries" {
		t.Fatal("saved status trusted")
	}
	value := "0-3"
	fact := p.Facts["after/thread/11"]
	fact.Value = &value
	p.Facts["after/thread/11"] = fact
	if status, _ := CheckCPUPartition(p); status != "not_ready" {
		t.Fatal("overlapping collector accepted", status)
	}
}

func TestCPUPartitionRefusals(t *testing.T) {
	for _, tc := range []struct{ name, file, value, status string }{
		{"member", "sys/fs/cgroup/measurement/cpuset.cpus.partition", "member", "not_ready"},
		{"invalid partition", "sys/fs/cgroup/measurement/cpuset.cpus.partition", "isolated invalid (no cpu)", "not_ready"},
		{"root not isolated", "sys/fs/cgroup/measurement/cpuset.cpus.partition", "root", "not_ready"},
		{"narrowed effective", "sys/fs/cgroup/measurement/cpuset.cpus.effective", "2", "not_ready"},
		{"widened exclusive", "sys/fs/cgroup/measurement/cpuset.cpus.exclusive.effective", "0-3", "not_ready"},
		{"populated", "sys/fs/cgroup/measurement/cgroup.events", "populated 1", "not_ready"},
		{"duplicate population", "sys/fs/cgroup/measurement/cgroup.events", "populated 0\npopulated 0", "not_ready"},
		{"missing population", "sys/fs/cgroup/measurement/cgroup.events", "frozen 0", "not_ready"},
		{"offline", "sys/devices/system/cpu/online", "0-2", "not_ready"},
		{"sibling outside", "sys/devices/system/cpu/cpu2/topology/thread_siblings_list", "0,2", "not_ready"},
		{"self absent", "sys/devices/system/cpu/cpu2/topology/thread_siblings_list", "3", "invalid_evidence"},
		{"collector overlap", "proc/self/task/11/status", "Cpus_allowed_list: 0-3", "not_ready"},
		{"mask absent", "proc/self/task/11/status", "Name: controller", "unavailable"},
		{"malformed mask", "proc/self/task/11/status", "Cpus_allowed_list: 1-0", "unavailable"},
		{"oversized fact", "sys/fs/cgroup/measurement/cpuset.cpus.partition", strings.Repeat("x", (64<<10)+1), "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := partitionFixture()
			root[tc.file].Data = []byte(tc.value)
			p := readCPUPartition(root, newCPUPartitionProbe("/sys/fs/cgroup/measurement", "2-3"))
			if p.Status != tc.status {
				t.Fatal(p.Status, p.Reason)
			}
		})
	}
	root := partitionFixture()
	delete(root, "sys/fs/cgroup/measurement/cpuset.cpus.exclusive.effective")
	p := readCPUPartition(root, newCPUPartitionProbe("/sys/fs/cgroup/measurement", "2-3"))
	if p.Status != "unavailable" || p.Facts["before/cpuset.cpus.exclusive.effective"].Status != "not_exposed" {
		t.Fatal(p)
	}
}

type changingPartitionFS struct {
	fstest.MapFS
	opens       int
	file, value string
}

func (f *changingPartitionFS) Open(name string) (fs.File, error) {
	if name == f.file {
		f.opens++
		if f.opens == 2 {
			if name == "proc/self/task" {
				f.MapFS["proc/self/task/12/status"] = &fstest.MapFile{Data: []byte("Cpus_allowed_list: 0")}
			} else {
				f.MapFS[name].Data = []byte(f.value)
			}
		}
	}
	return f.MapFS.Open(name)
}
func TestCPUPartitionChangesDuringProbe(t *testing.T) {
	for _, tc := range []struct{ file, value, want string }{
		{"sys/fs/cgroup/measurement/cpuset.cpus.partition", "member", "not_ready"},
		{"sys/devices/system/cpu/online", "0-4", "changed_during_probe"},
		{"proc/self/task/11/status", "Cpus_allowed_list: 2", "not_ready"},
		{"proc/self/task", "", "changed_during_probe"},
	} {
		root := &changingPartitionFS{MapFS: partitionFixture(), file: tc.file, value: tc.value}
		p := readCPUPartition(root, newCPUPartitionProbe("/sys/fs/cgroup/measurement", "2-3"))
		if p.Status != tc.want {
			t.Fatal(p.Status, p.Reason)
		}
	}
}

func TestCPUPartitionRequestAndPortability(t *testing.T) {
	if p := ProbeCPUPartition("", ""); p.Status != "not_requested" {
		t.Fatal(p)
	}
	for _, request := range [][2]string{{"relative", "2"}, {"/", "2"}, {"/sys/fs/cgroup/x/../y", "2"}, {"/sys/fs/cgroup/x", ""}, {"", "2"}, {"/sys/fs/cgroup/x", "0-99999999"}} {
		if p := ProbeCPUPartition(request[0], request[1]); p.Status != "invalid_request" {
			t.Fatal(p)
		}
	}
	if runtime.GOOS != "linux" {
		if p := ProbeCPUPartition("/sys/fs/cgroup/measurement", "2-3"); p.Status != "unsupported" {
			t.Fatal(p)
		}
	}
}
