package corpus

import (
	"os"
	"testing"
)

func TestCallsCorpusIsSourceBuiltAndHasOppositeDirections(t *testing.T) {
	workloads, err := Generate(t.TempDir(), "calls")
	if err != nil {
		t.Fatal(err)
	}
	if len(workloads) != 2 {
		t.Fatalf("got %d calls workloads", len(workloads))
	}
	if workloads[0].WorkUnit != "host-to-wasm_call" || workloads[0].HostProfile != "" || workloads[1].WorkUnit != "wasm-to-host_call" || workloads[1].HostProfile != "identity-v1" {
		t.Fatalf("unexpected directions or callback contract: %+v", workloads)
	}
	for _, workload := range workloads {
		bytes, err := os.ReadFile(workload.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		if Hash(bytes) != workload.SHA256 {
			t.Fatalf("source-built module digest differs: %s", workload.ID)
		}
		if _, err = Analyze(bytes); err != nil {
			t.Fatalf("invalid generated module %s: %v", workload.ID, err)
		}
	}
}
