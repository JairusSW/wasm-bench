package agent

import (
	"testing"
	"testing/fstest"
	"time"
)

const activeParent = "/sys/fs/cgroup/measurement"
const activeWorker = activeParent + "/wasmbench-123"

func activePartitionFixture() fstest.MapFS {
	root := partitionFixture()
	root["sys/fs/cgroup/measurement/cgroup.events"].Data = []byte("populated 1\n")
	for name, value := range map[string]string{
		"sys/fs/cgroup/measurement/cgroup.procs":                        "",
		"sys/fs/cgroup/measurement/wasmbench-123/cgroup.events":         "populated 1\n",
		"sys/fs/cgroup/measurement/wasmbench-123/cgroup.procs":          "123\n",
		"sys/fs/cgroup/measurement/wasmbench-123/cpuset.cpus.effective": "2-3\n",
	} {
		root[name] = &fstest.MapFile{Data: []byte(value)}
	}
	return root
}

func TestActivePartitionSampleAndTamper(t *testing.T) {
	root := activePartitionFixture()
	s := readActivePartition(root, activeParent, activeWorker, "2-3", 123, time.Now().UTC())
	if s.Status != "ready_at_sample" {
		t.Fatal(s.Status, s.Reason)
	}
	if err := ValidateActivePartitionSample(s); err != nil {
		t.Fatal(err)
	}
	m := &PartitionMonitor{Version: ActivePartitionVersion, IntervalNS: partitionSampleInterval.Nanoseconds(), Status: "ready_at_samples", Reason: "all recorded samples show the single adapter worker; unsampled intervals remain unqualified", Samples: []ActivePartitionSample{s, s}}
	if err := ValidatePartitionMonitor(m); err != nil {
		t.Fatal(err)
	}
	m.Samples[1].Status = "forged"
	if err := ValidatePartitionMonitor(m); err == nil {
		t.Fatal("forged sample verdict accepted")
	}
}

func TestActivePartitionRejectsIntrudersAndDrift(t *testing.T) {
	for _, tc := range []struct {
		name, file, value, want string
	}{
		{"partition mode", "sys/fs/cgroup/measurement/cpuset.cpus.partition", "member", "not_ready"},
		{"parent intruder", "sys/fs/cgroup/measurement/cgroup.procs", "456", "not_ready"},
		{"worker intruder", "sys/fs/cgroup/measurement/wasmbench-123/cgroup.procs", "123\n456", "not_ready"},
		{"worker left", "sys/fs/cgroup/measurement/wasmbench-123/cgroup.events", "populated 0", "not_ready"},
		{"cpu mask changed", "sys/fs/cgroup/measurement/wasmbench-123/cpuset.cpus.effective", "2", "not_ready"},
		{"cpu offline", "sys/devices/system/cpu/online", "0-2", "not_ready"},
		{"controller overlap", "proc/self/task/10/status", "Cpus_allowed_list: 0-3", "not_ready"},
		{"collector overlap", "proc/self/task/11/status", "Cpus_allowed_list: 2", "not_ready"},
		{"collector missing mask", "proc/self/task/11/status", "Name: collector", "unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := activePartitionFixture()
			root[tc.file].Data = []byte(tc.value)
			s := readActivePartition(root, activeParent, activeWorker, "2-3", 123, time.Now().UTC())
			if s.Status != tc.want {
				t.Fatal(s.Status, s.Reason)
			}
			if err := ValidateActivePartitionSample(s); err != nil {
				t.Fatal(err)
			}
		})
	}
	root := activePartitionFixture()
	root["sys/fs/cgroup/measurement/other/cgroup.events"] = &fstest.MapFile{Data: []byte("populated 1\n")}
	s := readActivePartition(root, activeParent, activeWorker, "2-3", 123, time.Now().UTC())
	if s.Status != "not_ready" || s.Reason != "another child cgroup is populated" {
		t.Fatal(s.Status, s.Reason)
	}
	delete(root, "sys/fs/cgroup/measurement/wasmbench-123/cgroup.procs")
	s = readActivePartition(root, activeParent, activeWorker, "2-3", 123, time.Now().UTC())
	if s.Status != "unavailable" {
		t.Fatal(s.Status, s.Reason)
	}
}

func TestActivePartitionControllerInventoryDrift(t *testing.T) {
	root := &changingPartitionFS{MapFS: activePartitionFixture(), file: "proc/self/task"}
	s := readActivePartition(root, activeParent, activeWorker, "2-3", 123, time.Now().UTC())
	if s.Status != "not_ready" || s.Reason != "controller thread inventory changed during sample" {
		t.Fatal(s.Status, s.Reason)
	}
	if err := ValidateActivePartitionSample(s); err != nil {
		t.Fatal(err)
	}
}

func TestActivePartitionControllerReceiptTamper(t *testing.T) {
	for _, change := range []func(map[string]HostFact){
		func(f map[string]HostFact) { delete(f, "controller/after/threads") },
		func(f map[string]HostFact) { delete(f, "controller/before/thread/10") },
		func(f map[string]HostFact) {
			v := "10,10"
			x := f["controller/before/threads"]
			x.Value = &v
			f["controller/before/threads"] = x
		},
		func(f map[string]HostFact) {
			v := "2"
			x := f["controller/after/thread/11"]
			x.Value = &v
			f["controller/after/thread/11"] = x
		},
	} {
		s := readActivePartition(activePartitionFixture(), activeParent, activeWorker, "2-3", 123, time.Now().UTC())
		change(s.Facts)
		if err := ValidateActivePartitionSample(s); err == nil {
			t.Fatal("forged controller receipt accepted")
		}
	}
}

func TestActivePartitionLegacyContractDoesNotAcquireControllerQualification(t *testing.T) {
	s := readActivePartition(activePartitionFixture(), activeParent, activeWorker, "2-3", 123, time.Now().UTC())
	s.Version, s.Scope = LegacyActivePartitionVersion, LegacyActivePartitionScope
	for key := range s.Facts {
		if len(key) >= len("controller/") && key[:len("controller/")] == "controller/" {
			delete(s.Facts, key)
		}
	}
	s.Status, s.Reason = CheckActivePartition(s)
	if s.Status != "ready_at_sample" || ValidateActivePartitionSample(s) != nil {
		t.Fatal("legacy evidence became unreadable", s)
	}
	m := &PartitionMonitor{Version: LegacyActivePartitionVersion, IntervalNS: partitionSampleInterval.Nanoseconds(), Status: "ready_at_samples", Reason: "all recorded samples show the single adapter worker; unsampled intervals remain unqualified", Samples: []ActivePartitionSample{s}}
	if err := ValidatePartitionMonitor(m); err != nil {
		t.Fatal(err)
	}
	m.Version = ActivePartitionVersion
	if ValidatePartitionMonitor(m) == nil {
		t.Fatal("legacy samples accepted under new monitor contract")
	}
}

func TestActivePartitionRequestBoundary(t *testing.T) {
	for _, worker := range []string{"/", activeParent, activeParent + "/elsewhere", activeParent + "/x/wasmbench-123", activeParent + "/../wasmbench-123"} {
		if err := activePartitionRequest(activeParent, worker, "2-3", 123); err == nil {
			t.Fatal("accepted worker", worker)
		}
	}
}
