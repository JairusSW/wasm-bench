package protocol

import "testing"

func TestComponentU64Contract(t *testing.T) {
	w := Workload{ABI: "component", HostProfile: ComponentU64Policy, Export: "benchmark", Args: Values{7}, WorkUnit: "invocation", Units: 1, Reset: "fresh_instance_per_sample", Oracle: Oracle{Kind: "exact_u64", Expected: Values{8}}}
	if err := ValidateComponentU64Workload(w); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*Workload){
		func(w *Workload) { w.HostProfile = "" }, func(w *Workload) { w.ABI = "core" }, func(w *Workload) { w.Reset = "stateless" }, func(w *Workload) { w.Export = "" }, func(w *Workload) { w.Args = make(Values, 17) }, func(w *Workload) { w.Oracle.Expected = nil }, func(w *Workload) { w.Oracle.Expected = Values{1, 2} }, func(w *Workload) { w.Command = &CommandContract{} }, func(w *Workload) { w.Initialize = "init" }, func(w *Workload) { w.Input = &MemoryInput{} },
	} {
		bad := w
		change(&bad)
		if ValidateComponentU64Workload(bad) == nil {
			t.Fatal("invalid contract accepted")
		}
	}
}
