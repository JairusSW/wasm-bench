package corpus

import (
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
)

func generateGuestDensity(root string) ([]protocol.Workload, error) {
	var out []protocol.Workload
	for _, pages := range []uint32{1, 4} {
		b, err := CheckpointModule(pages)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(root, "guest-density", fmt.Sprintf("memory-%d.wasm", pages))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			return nil, err
		}
		for _, policy := range []string{"fresh_initialize", "eager_guest_restore"} {
			for _, write := range []bool{false, true} {
				for _, count := range []int{1, 4, 16, 64} {
					state := "unchanged"
					want := uint64(pages)*65536*7 + 42
					if write {
						state = "first_write"
						want += 5
					}
					w := protocol.Workload{Schema: 1, ID: fmt.Sprintf("guest-density/%s/%s/%d-pages/%d", policy, state, pages, count), Family: "guest-density", Artifact: path, SHA256: Hash(b), ABI: "core", Features: []string{"mvp"}, Export: "benchmark", WorkUnit: "instance_group", Units: 1, Reset: "fresh_instance_per_sample", Oracle: protocol.Oracle{Kind: "guest_density_v1", Expected: protocol.Values{want}}, License: "MIT", Source: "corpus/checkpoint.go", Generator: "wasmbench-guest-density-v1", Dimension: "instances", Size: count, GuestDensity: &protocol.GuestDensityContract{Instances: count, Pages: pages, Provisioning: policy, FirstWrite: write}}
					w.Generator = protocol.GuestDensityGenerator(*w.GuestDensity)
					if err := protocol.ValidateGuestDensityWorkload(w); err != nil {
						return nil, err
					}
					out = append(out, w)
				}
			}
		}
	}
	return out, nil
}
