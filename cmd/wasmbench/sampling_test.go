package main

import (
	"reflect"
	"testing"
)

func TestDefaultScenarioSamples(t *testing.T) {
	scenarios := []string{"compile", "instantiate", "first-call", "steady"}
	got := defaultScenarioSamples("timing", scenarios, nil, false, false)
	want := map[string]int{"*": 1, "compile": 3, "instantiate": 3, "steady": 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v; want %v", got, want)
	}
	// A selected subset must not acquire keys rejected by lock validation.
	got = defaultScenarioSamples("timing", []string{"compile"}, nil, false, false)
	if !reflect.DeepEqual(got, map[string]int{"*": 1, "compile": 3}) {
		t.Fatal(got)
	}
	for _, profile := range []string{"memory", "code", "counters", "profiling"} {
		if got := defaultScenarioSamples(profile, scenarios, nil, false, false); got != nil {
			t.Fatalf("%s: %v", profile, got)
		}
	}
	if got := defaultScenarioSamples("timing", scenarios, nil, true, false); got != nil {
		t.Fatal("explicit uniform sample count overridden")
	}
	explicit := map[string]int{"steady": 2}
	for _, samplesExplicit := range []bool{false, true} {
		if got := defaultScenarioSamples("timing", scenarios, explicit, samplesExplicit, true); !reflect.DeepEqual(got, explicit) {
			t.Fatal("explicit scenario counts overridden")
		}
	}
}
