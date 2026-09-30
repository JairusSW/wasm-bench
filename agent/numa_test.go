package agent

import "testing"

func TestNUMARequestAndReadback(t *testing.T) {
	for _, nodes := range []string{"2-1", "1,1", "1-3,2", "0-4096", "-1", "1,,2"} {
		if (ResourcePolicy{CgroupParent: "/cg", Mems: nodes}).Validate() == nil {
			t.Fatal(nodes)
		}
	}
	if (ResourcePolicy{Mems: "0"}).Validate() == nil {
		t.Fatal("NUMA without cgroup accepted")
	}
	p := ResourcePolicy{CgroupParent: "/cg", Mems: "2,0-1"}
	values := map[string]string{"cpuset.mems": "0-2", "cpuset.mems.effective": "2,1,0"}
	first := CheckResourceReadback(p, values, "before_spawn")
	last := CheckResourceReadback(p, values, "response_end_before_cleanup")
	iso := &Isolation{Mode: "cgroup_v2_at_spawn", Path: "/cg/leaf", Effective: values, Verification: &first, FinalVerification: &last}
	if err := ValidateNUMAIsolation(p, iso, "response_end_before_cleanup"); err != nil {
		t.Fatal(err)
	}
	for _, actual := range []string{"0-1", "0-3", "", "unavailable: denied"} {
		values["cpuset.mems.effective"] = actual
		if CheckResourceReadback(p, values, "end").Err() == nil {
			t.Fatal("bad grant accepted", actual)
		}
	}
	values["cpuset.mems.effective"] = "2,1,0"
	iso.FinalVerification = nil
	if ValidateNUMAIsolation(p, iso, "response_end_before_cleanup") == nil {
		t.Fatal("missing endpoint accepted")
	}
	iso.FinalVerification = &last
	last.Stage = "tool_exit_before_cleanup"
	if ValidateNUMAIsolation(p, iso, "response_end_before_cleanup") == nil {
		t.Fatal("wrong boundary accepted")
	}
	last = CheckResourceReadback(p, values, "response_end_before_cleanup")
	last.Effective["cpuset.mems.effective"] = "0"
	if ValidateNUMAIsolation(p, iso, "response_end_before_cleanup") == nil {
		t.Fatal("forged verified status accepted")
	}
}
