//go:build linux

package agent

import (
	"os"
	"testing"
)

func TestCPUPartitionRejectsOrdinaryFilesystem(t *testing.T) {
	p := ProbeCPUPartition(t.TempDir(), "0")
	if p.Status != "invalid_request" {
		t.Fatal(p)
	}
}

func TestCPUPartitionLiveReadOnly(t *testing.T) {
	partition, cpus := os.Getenv("WASMBENCH_CPU_PARTITION"), os.Getenv("WASMBENCH_MEASUREMENT_CPUS")
	if partition == "" || cpus == "" {
		t.Skip("set WASMBENCH_CPU_PARTITION and WASMBENCH_MEASUREMENT_CPUS for an existing empty isolated partition; probe never changes settings")
	}
	p := ProbeCPUPartition(partition, cpus)
	if p.Status != "ready_at_observed_boundaries" {
		t.Fatalf("%s: %s", p.Status, p.Reason)
	}
}
