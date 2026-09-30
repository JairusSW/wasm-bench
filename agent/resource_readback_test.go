package agent

import "testing"

func TestRequestedResourceReadback(t *testing.T) {
	p := ResourcePolicy{CgroupParent: "/cg", MemoryMaxBytes: 4096, DisableSwap: true, CPUQuotaUS: 100000, CPUs: "3,1-2", PidsMax: 64}
	valid := map[string]string{"memory.max": "4096", "memory.oom.group": "1", "memory.swap.max": "0", "cpu.max": "100000  100000", "cpuset.cpus": "1-3", "cpuset.cpus.effective": "1,2,3", "pids.max": "64"}
	v := CheckResourceReadback(p, valid, "before_spawn")
	if v.Status != "verified" || v.Err() != nil {
		t.Fatal(v)
	}
	valid["memory.max"] = "1"
	if v.Effective["memory.max"] != "4096" {
		t.Fatal("evidence aliases read buffer")
	}
	valid["memory.max"] = "4096"
	for _, key := range []string{"memory.max", "memory.oom.group", "memory.swap.max", "cpu.max", "cpuset.cpus", "cpuset.cpus.effective", "pids.max"} {
		original := valid[key]
		for _, value := range []string{"", "unavailable: permission denied", "999"} {
			valid[key] = value
			bad := CheckResourceReadback(p, valid, "response_end_before_cleanup")
			if bad.Status == "verified" || bad.Err() == nil {
				t.Fatal(key, value, bad)
			}
		}
		delete(valid, key)
		if CheckResourceReadback(p, valid, "end").Err() == nil {
			t.Fatal("missing readback accepted", key)
		}
		valid[key] = original
	}
	valid["cpuset.cpus.effective"] = "1-2"
	if v := CheckResourceReadback(p, valid, "end"); v.Status != "mismatch" {
		t.Fatal("shrunk effective cpuset accepted", v)
	}
	valid["cpuset.cpus.effective"] = "1-4"
	if v := CheckResourceReadback(p, valid, "end"); v.Status != "mismatch" {
		t.Fatal("expanded effective cpuset accepted", v)
	}
	if v := CheckResourceReadback(ResourcePolicy{CgroupParent: "/cg"}, nil, "start"); v.Status != "not_requested" {
		t.Fatal(v)
	}
}

func TestResourceCPUListsAreBoundedAndExact(t *testing.T) {
	for _, list := range []string{"3-1", "1,1", "1-3,2", "0-4096", "2147483648", "1,,2"} {
		if (ResourcePolicy{CgroupParent: "/cg", CPUs: list}).Validate() == nil {
			t.Fatal("invalid list accepted", list)
		}
	}
}
