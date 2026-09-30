package metrics

import (
	"github.com/wasmbench/wasmbench/protocol"
	"testing"
)

func TestProcessSnapshotScopesRemainDistinct(t *testing.T) {
	for _, id := range protocol.ProcessSnapshotScenarios() {
		found := false
		for _, s := range Scenarios {
			if s.ID == id {
				found = true
				want := "embedding_api"
				if id == "process-snapshot-capture" || id == "process-snapshot-restore" {
					want = "process_control_roundtrip"
				}
				if s.Scope != want || s.Boundary == "" {
					t.Fatal("snapshot stage scope is ambiguous")
				}
			}
		}
		if !found {
			t.Fatal("snapshot stage has no metric definition")
		}
	}
}

func TestVersionedRegistryIsCompleteAndUnique(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryRejectsAmbiguousContracts(t *testing.T) {
	metric := Definition{Name: "time.wall", Version: 1, Unit: "ns", Scope: "embedding_api", Boundary: "monotonic timed call batch", MissingPolicy: "unavailable"}
	scenario := Scenario{ID: "first-call", Boundary: "fresh instance invocation", Scope: "embedding_api"}
	tests := []struct {
		name        string
		definitions []Definition
		scenarios   []Scenario
	}{
		{"empty", nil, nil},
		{"duplicate metric", []Definition{metric, metric}, []Scenario{scenario}},
		{"missing metric boundary", []Definition{{Name: "time.wall", Version: 1, Unit: "ns", Scope: "embedding_api", MissingPolicy: "unavailable"}}, []Scenario{scenario}},
		{"duplicate scenario", []Definition{metric}, []Scenario{scenario, scenario}},
		{"missing scenario scope", []Definition{metric}, []Scenario{{ID: "first-call", Boundary: "one call"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateDefinitions(tt.definitions, tt.scenarios); err == nil {
				t.Fatal("accepted invalid registry")
			}
		})
	}
}
