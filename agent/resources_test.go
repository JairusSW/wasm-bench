package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestResourcePolicyValidation(t *testing.T) {
	for _, p := range []ResourcePolicy{{MemoryMaxBytes: 1}, {CPUQuotaUS: 1000}, {CPUs: "0"}, {PidsMax: 1}, {CgroupParent: "relative"}, {CgroupParent: "/cg", CPUs: "0;1"}, {CgroupParent: "/cg", CPUQuotaUS: 1}} {
		if p.Validate() == nil {
			t.Fatalf("invalid policy accepted: %+v", p)
		}
	}
	if err := (ResourcePolicy{CgroupParent: "/cg", CPUs: "0-3,5", CPUQuotaUS: 100000}).Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestedIsolationNeverFallsBack(t *testing.T) {
	c, err := StartIsolated(context.Background(), []string{"sleep", "10"}, filepath.Join(t.TempDir(), "log"), time.Second, ResourcePolicy{CgroupParent: t.TempDir()})
	if c != nil {
		c.Close()
		t.Fatal("launched without a cgroup v2 filesystem")
	}
	if err == nil {
		t.Fatal("no error for missing isolation")
	}
}
