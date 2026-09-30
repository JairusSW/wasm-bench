package protocol

import "fmt"

// ValidateAppInit requires fresh core-module state and the existing scalar
// workload oracle. Input installation follows initialization outside timing.
func ValidateAppInit(w Workload) error {
	if w.ABI == "wasi-reactor" {
		return ValidateReactor(w)
	}
	if w.Initialize == "" || w.ABI != "core" || w.Command != nil || w.Vectors != nil || w.Oracle.Kind != "exact_u64" || (w.Reset != "stateless" && w.Reset != "fresh_instance_per_sample") {
		return fmt.Errorf("app-init requires a declared initializer and a core scalar oracle")
	}
	return nil
}
