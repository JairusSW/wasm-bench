//go:build linux

package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/collectors"
)

func TestLinuxNUMAReadbackDrift(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cpuset.mems", "cpuset.mems.effective"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("0-1"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	g := &cgroup{dir: dir, policy: ResourcePolicy{CgroupParent: "/cg", Mems: "1,0"}}
	first := g.verifyResources("before_spawn")
	if first.Status != "verified" {
		t.Fatal(first)
	}
	if err := os.WriteFile(filepath.Join(dir, "cpuset.mems.effective"), []byte("0"), 0600); err != nil {
		t.Fatal(err)
	}
	if last := g.verifyResources("end"); last.Status != "mismatch" || first.Effective["cpuset.mems.effective"] != "0-1" {
		t.Fatal(first, last)
	}
}

func TestCgroupNUMAAtSpawn(t *testing.T) {
	parent := os.Getenv("WASMBENCH_TEST_CGROUP_PARENT")
	if parent == "" {
		t.Skip("explicit delegated cgroup required")
	}
	data, err := os.ReadFile(filepath.Join(parent, "cpuset.mems.effective"))
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := collectors.ParseCPUList(string(data))
	if err != nil {
		t.Fatal(err)
	}
	requested := strconv.Itoa(nodes[0])
	cmd := exec.Command("/bin/sh", "-c", `awk '/^Mems_allowed_list:/ {print $2}' /proc/self/status`)
	g, err := prepareCgroup(cmd, ResourcePolicy{CgroupParent: parent, Mems: requested})
	if err != nil {
		t.Fatal(err)
	}
	defer g.close()
	if g.verification.Status != "verified" {
		t.Fatal(g.verification)
	}
	actual, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(actual)) != requested {
		t.Fatal("wrong task memory-node allowance", string(actual), err)
	}
	if v := g.verifyResources("tool_exit_before_cleanup"); v.Status != "verified" {
		t.Fatal(v)
	}
}
