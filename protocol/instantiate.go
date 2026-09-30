package protocol

import "fmt"

// ValidateInstantiatePhases currently admits core scalar contracts. An explicit
// initializer is optional and runs only after the instantiated snapshot.
func ValidateInstantiatePhases(w Workload) error {
	if w.ABI != "core" || w.Command != nil || w.Vectors != nil || (w.Oracle.Kind != "exact_u64" && w.Oracle.Kind != "float_bits_v1") || (w.Reset != "stateless" && w.Reset != "fresh_instance_per_sample") {
		return fmt.Errorf("instantiation barriers require a core scalar oracle")
	}
	if w.Oracle.Kind == "float_bits_v1" {
		return ValidateFloatWorkload(w)
	}
	return nil
}
