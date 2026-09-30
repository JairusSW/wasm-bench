package protocol

import (
	"strings"
	"testing"
)

func TestRustAllocatorWorkloadResetAndOracleContracts(t *testing.T) {
	for _, oracle := range []string{"exact_u64", "float_bits_v1"} {
		for _, reset := range []string{"stateless", "fresh_instance_per_sample", "unknown"} {
			for _, scenario := range []string{"compile", "instantiate", "first-call", "steady"} {
				w := Workload{ABI: "core", Reset: reset, Oracle: Oracle{Kind: oracle}}
				valid := reset == "stateless" || (reset == "fresh_instance_per_sample" && scenario != "steady")
				if (ValidateRustAllocatorWorkload(w, scenario) == nil) != valid {
					t.Fatal(oracle, reset, scenario)
				}
			}
		}
	}
	for _, mutate := range []func(*Workload){
		func(w *Workload) { w.ABI = "wasi-command" },
		func(w *Workload) { w.Oracle.Kind = "exact_vectors" },
		func(w *Workload) { w.Command = &CommandContract{} },
		func(w *Workload) { w.Density = &DensityContract{} },
		func(w *Workload) { w.Checkpoint = &CheckpointContract{} },
		func(w *Workload) { w.GuestDensity = &GuestDensityContract{} },
	} {
		w := Workload{ABI: "core", Reset: "stateless", Oracle: Oracle{Kind: "exact_u64"}}
		mutate(&w)
		if ValidateRustAllocatorWorkload(w, "compile") == nil {
			t.Fatal("unsupported compound workload accepted", w)
		}
	}
}

func rustAllocatorFixture() Sample {
	s := Sample{Operations: 1, SampleType: "individual_operation", Verified: true}
	for _, pair := range []struct {
		metric string
		value  float64
	}{{"host.rust.alloc.bytes", 100}, {"host.rust.alloc.count", 2}, {"host.rust.freed.bytes", 60}, {"host.rust.outstanding.start", 20}, {"host.rust.outstanding.end", 60}, {"host.rust.outstanding.observed_peak", 110}} {
		unit := "bytes"
		if pair.metric == "host.rust.alloc.count" {
			unit = "count"
		}
		s.Observations = append(s.Observations, Observation{Metric: pair.metric, Value: Value(pair.value), Unit: unit, DefinitionVersion: 1, Scope: RustAllocatorScope, Phase: "compile", Collector: RustAllocatorCollector, CollectorVersion: RustAllocatorVersion, Quality: "instrumented", Profile: "memory", Status: "available", Denominator: "single_embedding_api_operation"})
	}
	return s
}

func releaseFixture(retained bool) Sample {
	s := rustAllocatorFixture()
	for _, pair := range []struct {
		suffix, unit string
		value        float64
	}{{"alloc.bytes", "bytes", 0}, {"alloc.count", "count", 0}, {"freed.bytes", "bytes", 60}, {"outstanding.start", "bytes", 80}, {"outstanding.end", "bytes", 20}, {"outstanding.observed_peak", "bytes", 80}, {"elapsed", "ns", 0}} {
		o := Observation{Metric: "host.rust.release." + pair.suffix, Value: Value(pair.value), Unit: pair.unit, DefinitionVersion: 1, Scope: RustAllocatorScope, Phase: "steady/logical_release_window", Collector: RustAllocatorCollector, CollectorVersion: RustAllocatorVersion, Quality: "instrumented", Profile: "memory", Status: "available", Denominator: "single_logical_release_operation"}
		if retained {
			o.Value = nil
			o.Status = "not_applicable"
			o.Reason = "steady Store retained across samples; release on final sample"
		}
		s.Observations = append(s.Observations, o)
	}
	return s
}

func TestRustReleaseKeepsVerificationGapAndRetainedSamplesDistinct(t *testing.T) {
	for _, mode := range []string{"released", "retained", "missing", "duplicate", "wrong-phase", "wrong-domain", "wrong-denominator", "wrong-unit", "net", "peak", "fraction", "pretend-zero", "missing-final", "false-retained", "negative-time"} {
		t.Run(mode, func(t *testing.T) {
			retained := mode == "retained" || mode == "pretend-zero" || mode == "missing-final"
			s := releaseFixture(retained)
			final := !retained || mode == "missing-final"
			switch mode {
			case "missing":
				s.Observations = s.Observations[:12]
			case "duplicate":
				s.Observations = append(s.Observations, s.Observations[6])
			case "wrong-phase":
				s.Observations[6].Phase = "steady"
			case "wrong-domain":
				s.Observations[6].Scope = "process_rss"
			case "wrong-denominator":
				s.Observations[6].Denominator = "single_embedding_api_operation"
			case "wrong-unit":
				s.Observations[12].Unit = "bytes"
			case "net":
				s.Observations[10].Value = Value(21)
			case "peak":
				s.Observations[11].Value = Value(79)
			case "fraction":
				s.Observations[12].Value = Value(0.5)
			case "pretend-zero":
				for i := 6; i < len(s.Observations); i++ {
					s.Observations[i].Value = Value(0)
				}
			case "false-retained":
				final = false
			case "negative-time":
				s.Observations[12].Value = Value(-1)
			}
			err := ValidateRustAllocatorRelease(s, "steady", final)
			if (err == nil) != (mode == "released" || mode == "retained") {
				t.Fatal(mode, err)
			}
			// The API validator must not conflate distinct release metrics.
			if mode == "released" {
				if err := ValidateRustAllocatorSample(s, "compile"); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
	for _, scenario := range []string{"compile", "instantiate", "first-call"} {
		s := releaseFixture(false)
		for i := range s.Observations {
			if strings.HasPrefix(s.Observations[i].Metric, "host.rust.release.") {
				s.Observations[i].Phase = scenario + "/logical_release_window"
			}
		}
		if err := ValidateRustAllocatorRelease(s, scenario, false); err != nil {
			t.Fatal(scenario, err)
		}
	}
}

func TestRustAllocatorEvidencePreservesDomainAndArithmetic(t *testing.T) {
	for _, mode := range []string{"valid", "zero", "missing", "duplicate", "scope", "collector", "profile", "quality", "phase", "denominator", "fraction", "precision", "net", "peak-low", "peak-high", "warmup", "batch", "unverified"} {
		t.Run(mode, func(t *testing.T) {
			s := rustAllocatorFixture()
			switch mode {
			case "zero":
				for i := range s.Observations {
					s.Observations[i].Value = Value(0)
				}
			case "missing":
				s.Observations = s.Observations[1:]
			case "duplicate":
				s.Observations = append(s.Observations, s.Observations[0])
			case "scope":
				s.Observations[0].Scope = "all_host_allocations"
			case "collector":
				s.Observations[0].CollectorVersion = "unknown"
			case "profile":
				s.Observations[0].Profile = "timing"
			case "quality":
				s.Observations[0].Quality = "kernel_accounted_peak"
			case "phase":
				s.Observations[0].Phase = "instantiate"
			case "denominator":
				s.Observations[0].Denominator = "batch"
			case "fraction":
				s.Observations[0].Value = Value(1.5)
			case "precision":
				s.Observations[0].Value = Value(9007199254740992)
			case "net":
				s.Observations[4].Value = Value(61)
			case "peak-low":
				s.Observations[5].Value = Value(59)
			case "peak-high":
				s.Observations[5].Value = Value(121)
			case "warmup":
				s.Warmup = true
			case "batch":
				s.Operations = 2
			case "unverified":
				s.Verified = false
			}
			err := ValidateRustAllocatorSample(s, "compile")
			if (err == nil) != (mode == "valid" || mode == "zero") {
				t.Fatal(mode, err)
			}
		})
	}
}
