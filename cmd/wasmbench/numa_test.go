package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestNUMAPlanAndLockedPolicy(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{"bin/adapter-wazero", "adapters/wasmtime/target/release/wasm-analyze"} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("planning fixture only"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--cgroup-parent", "/delegated", "--cpus", "2-3", "--require-irq-affinity", "--out", "irq.lock"}); err != nil {
		t.Fatal(err)
	}
	var irqLock experiment.Lock
	if err := experiment.ReadJSON("irq.lock", &irqLock); err != nil {
		t.Fatal(err)
	}
	if !irqLock.RequireIRQAffinity {
		t.Fatal("IRQ requirement not locked")
	}
	if err := run(ctx, []string{"plan", "--lock", "irq.lock", "--out", "irq-copy.lock"}); err != nil {
		t.Fatal(err)
	}
	var irqCopy experiment.Lock
	if err := experiment.ReadJSON("irq-copy.lock", &irqCopy); err != nil {
		t.Fatal(err)
	}
	if !irqCopy.RequireIRQAffinity {
		t.Fatal("IRQ requirement lost on replay planning")
	}
	if err := run(ctx, []string{"plan", "--lock", "irq.lock", "--require-irq-affinity=false", "--out", "irq-bad.lock"}); err == nil {
		t.Fatal("IRQ override accepted")
	}
	if _, err := os.Stat("irq-bad.lock"); !os.IsNotExist(err) {
		t.Fatal("override wrote output")
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--require-irq-affinity", "--out", "irq-invalid.lock"}); err == nil {
		t.Fatal("IRQ requirement without CPU budget accepted")
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--cgroup-parent", "/delegated", "--mems", "0-1", "--out", "numa.lock"}); err != nil {
		t.Fatal(err)
	}
	var l experiment.Lock
	if err := experiment.ReadJSON("numa.lock", &l); err != nil {
		t.Fatal(err)
	}
	if l.Options.Resources.Mems != "0-1" {
		t.Fatal("NUMA policy lost", l.Options.Resources)
	}
	if err := run(ctx, []string{"plan", "--lock", "numa.lock", "--out", "copy.lock"}); err != nil {
		t.Fatal(err)
	}
	var copy experiment.Lock
	experiment.ReadJSON("copy.lock", &copy)
	if copy.Options.Resources != l.Options.Resources {
		t.Fatal("policy changed")
	}
	if run(ctx, []string{"plan", "--lock", "numa.lock", "--mems", "2", "--out", "bad.lock"}) == nil {
		t.Fatal("override accepted")
	}
	if _, err := os.Stat("bad.lock"); !os.IsNotExist(err) {
		t.Fatal("invalid plan written", err)
	}
	if run(ctx, []string{"plan", "--runtimes", "wazero", "--mems", "0", "--out", "unisolated.lock"}) == nil {
		t.Fatal("unisolated nodes accepted")
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--cgroup-parent", "/partition", "--cpus", "2-3", "--require-isolated-cpu-partition", "--out", "partition.lock"}); err != nil {
		t.Fatal(err)
	}
	var partition experiment.Lock
	if err := experiment.ReadJSON("partition.lock", &partition); err != nil {
		t.Fatal(err)
	}
	if !partition.RequireIsolatedCPUPartition {
		t.Fatal("partition requirement lost")
	}
	if err := run(ctx, []string{"plan", "--lock", "partition.lock", "--out", "partition-copy.lock"}); err != nil {
		t.Fatal(err)
	}
	var copied experiment.Lock
	if err := experiment.ReadJSON("partition-copy.lock", &copied); err != nil {
		t.Fatal(err)
	}
	if !copied.RequireIsolatedCPUPartition || copied.Options.Resources != partition.Options.Resources {
		t.Fatal("copied policy changed")
	}
	if err := run(ctx, []string{"plan", "--lock", "partition.lock", "--require-isolated-cpu-partition=false", "--out", "partition-override.lock"}); err == nil {
		t.Fatal("locked partition requirement overridden")
	}
	if err := run(ctx, []string{"plan", "--runtimes", "wazero", "--require-isolated-cpu-partition", "--out", "partition-invalid.lock"}); err == nil {
		t.Fatal("missing partition resources accepted")
	}
}
