package corpus

import (
	"encoding/base64"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

// Exact wat 1.251.0 output from the independently validated Wasmtime qualifier.
// Preserve bytes (including its name section); drift requires a new contract.
const processSnapshotFixture = "AGFzbQEAAAABCAJgAAF/YAAAAwkIAAABAQAAAAAEBQFwAQIEBQQBAQIEBgYBfwFBKgsHPQcGbWVtb3J5AgAEc2VlZAACBm11dGF0ZQADBWNoZWNrAAQFcHJvYmUABQVwYWdlcwAGCGVsZW1lbnRzAAcJDAIAQQALAgABAQABAAwBAQqXAQgEAEEHCwQAQQkLHABBAEEHOgAAQQDSASYAQQFAABrQcEEB/A8AGgsnAEEAQQs6AABB4wAkAEEA0gAmAEEBQAAa0HBBAfwPABr8CQD8DQELFwBBAC0AACMAakEAEQAAaj8AavwQAGoLIwBBgAFBAEED/AgAAEECQQBBAfwMAQBBgAEtAABBAhEAAGoLBAA/AAsFAPwQAAsLBgEBA3h5egAxBG5hbWUBBwIAAWEBAWIECQEABnJlc3VsdAUEAQABdAcEAQABZwgEAQEBZQkEAQABZA=="

func ProcessSnapshotModule() ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(processSnapshotFixture)
	if err != nil {
		return nil, err
	}
	if Hash(b) != protocol.ProcessSnapshotArtifactSHA256 {
		return nil, fmt.Errorf("canonical process snapshot fixture drift")
	}
	return b, nil
}

func generateProcessSnapshots(root string) ([]protocol.Workload, error) {
	b, err := ProcessSnapshotModule()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, "process-snapshots", "quiescent-core.wasm")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0644); err != nil {
		return nil, err
	}
	w := protocol.Workload{Schema: 1, ID: "process-snapshots/quiescent-core", Family: "mechanisms", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"bulk-memory", "reference-types"}, Export: "check", Initialize: "seed", Reset: "fresh_process_snapshot_per_sample", WorkUnit: "process_snapshot_stage", Units: 1, Oracle: protocol.Oracle{Kind: "linux_process_snapshot_v1", Expected: protocol.Values{64}}, License: "MIT", Source: "corpus/process_snapshot.go", Generator: "wasmbench-process-snapshot-v1", ProcessSnapshot: &protocol.ProcessSnapshotContract{Mode: protocol.ProcessSnapshotMode}}
	return []protocol.Workload{w}, protocol.ValidateProcessSnapshotWorkload(w)
}

func generateSnapshotDensity(root string) ([]protocol.Workload, error) {
	base, err := generateProcessSnapshots(root)
	if err != nil {
		return nil, err
	}
	var workloads []protocol.Workload
	for _, count := range []int{1, 2, 4, 8, 32} {
		w := base[0]
		w.ID = fmt.Sprintf("process-snapshot-density/%d-instances", count)
		w.ProcessSnapshot = nil
		w.SnapshotDensity = &protocol.SnapshotDensityContract{Mode: "simultaneous_linux_process_cow", Instances: count}
		w.Dimension, w.Size = "instances", count
		w.Generator, w.WorkUnit, w.Reset, w.Oracle.Kind = "wasmbench-process-snapshot-density-v1", "restored_process_group", "fresh_process_snapshot_group_per_sample", "linux_process_snapshot_density_v1"
		if err := protocol.ValidateSnapshotDensityWorkload(w); err != nil {
			return nil, err
		}
		workloads = append(workloads, w)
	}
	return workloads, nil
}
