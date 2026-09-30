package protocol

import "fmt"

const ComponentU64Policy = "component-u64-v1"

// All parameters and the single result are Component Model u64 values, not
// untyped core-Wasm bit patterns. The adapter must check the actual signature.
func ValidateComponentU64Workload(w Workload) error {
	if w.ABI != "component" || w.HostProfile != ComponentU64Policy || w.Oracle.Kind != "exact_u64" || len(w.Oracle.Expected) != 1 || len(w.Args) > 16 || w.Export == "" || len(w.Export) > 256 || w.Reset != "fresh_instance_per_sample" || w.Units == 0 || w.WorkUnit == "" || w.Command != nil || w.Input != nil || w.Vectors != nil || w.Density != nil || w.Checkpoint != nil || w.GuestDensity != nil || w.Continuation != nil || w.ProcessSnapshot != nil || w.SnapshotDensity != nil || w.Initialize != "" || w.Oracle.Float != nil || w.Oracle.ExpectedTrap != "" || w.Oracle.OutputPointerExport != "" {
		return fmt.Errorf("invalid component-u64-v1 export-call contract")
	}
	return nil
}
