//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// Filesystem-injected tests exercise the Linux reader without changing host
// cgroups, requiring delegation, or pretending ordinary files enforce limits.
func TestLinuxResourceReadbackDrift(t *testing.T) {
	dir := t.TempDir()
	settings := map[string]string{"memory.max": "4096", "memory.oom.group": "1", "cpuset.cpus": "0-1", "cpuset.cpus.effective": "0-1"}
	for name, value := range settings {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	g := &cgroup{dir: dir, policy: ResourcePolicy{CgroupParent: "/cg", MemoryMaxBytes: 4096, CPUs: "0,1"}}
	first := g.verifyResources("before_spawn")
	if first.Status != "verified" {
		t.Fatal(first)
	}
	if err := os.WriteFile(filepath.Join(dir, "cpuset.cpus.effective"), []byte("1"), 0600); err != nil {
		t.Fatal(err)
	}
	last := g.verifyResources("response_end_before_cleanup")
	if last.Status != "mismatch" || first.Effective["cpuset.cpus.effective"] != "0-1" {
		t.Fatal(first, last)
	}
	if err := os.Remove(filepath.Join(dir, "memory.max")); err != nil {
		t.Fatal(err)
	}
	if last = g.verifyResources("end"); last.Status != "unavailable" {
		t.Fatal(last)
	}
}
