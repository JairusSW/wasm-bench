package protocol

import "testing"

func TestValidateInstantiatePhases(t *testing.T) {
	w := Workload{ABI: "core", Reset: "stateless", Oracle: Oracle{Kind: "exact_u64"}}
	if err := ValidateInstantiatePhases(w); err != nil {
		t.Fatal("initializer is optional", err)
	}
	for _, mutate := range []func(*Workload){
		func(w *Workload) { w.ABI = "wasi-command" },
		func(w *Workload) { w.Oracle.Kind = "unknown" },
		func(w *Workload) { w.Reset = "reused" },
		func(w *Workload) { w.Command = &CommandContract{} },
	} {
		invalid := w
		mutate(&invalid)
		if ValidateInstantiatePhases(invalid) == nil {
			t.Fatal("accepted invalid contract", invalid)
		}
	}
}
