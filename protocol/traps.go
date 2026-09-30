package protocol

import (
	"fmt"
	"slices"
	"time"
)

var TrapCodes = []string{"unreachable", "memory_out_of_bounds", "integer_divide_by_zero", "integer_overflow"}

type TrapResult struct {
	Code    string `json:"code"`
	Source  string `json:"source"`
	Message string `json:"message"`
}

func ValidateTrap(w Workload) error {
	if w.Oracle.Kind != "expected_trap" || !slices.Contains(TrapCodes, w.Oracle.ExpectedTrap) || w.ABI != "core" || w.Reset != "fresh_instance_per_sample" || w.Export == "" || w.HostProfile != "" || w.Initialize != "" || w.Input != nil || w.Command != nil || w.Vectors != nil || len(w.Args) != 0 || len(w.Oracle.Expected) != 0 || len(w.Oracle.Memory) != 0 || w.Oracle.OutputPointerExport != "" {
		return fmt.Errorf("unsupported or ambiguous invocation-trap contract")
	}
	return nil
}

func VerifyTrap(w Workload, result *TrapResult) error {
	if err := ValidateTrap(w); err != nil {
		return err
	}
	if result == nil || result.Code != w.Oracle.ExpectedTrap || result.Source == "" || result.Message == "" {
		return fmt.Errorf("incorrect result: expected invocation trap %s, got %+v", w.Oracle.ExpectedTrap, result)
	}
	return nil
}

// TrapFactory must finish construction and export lookup before returning.
// Only Invoke's error is eligible for the trap oracle, never setup or cleanup.
type TrapInstance struct {
	Invoke func() error
	Close  func()
	// BeginMemory starts an untimed diagnostic window and returns its snapshot
	// closure. The loop finishes it before classification and instance release.
	BeginMemory func() func() []Observation
}

type TrapFactory func() (TrapInstance, error)

func RunTrapSamples(p *Preparation, r *RunRequest, factory TrapFactory, classify func(error) *TrapResult) ([]Sample, error) {
	if p == nil || r == nil || factory == nil || classify == nil {
		return nil, fmt.Errorf("missing trap preparation")
	}
	if err := ValidateTrap(p.Workload); err != nil {
		return nil, err
	}
	if !slices.Contains([]string{"timing", "memory"}, p.Profile) || r.PhaseBarriers || !slices.Contains([]string{"first-call", "steady"}, r.Scenario) || r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 || r.Operations < 1 || r.Operations > 1000000 {
		return nil, fmt.Errorf("unsupported trap scenario/profile/batch")
	}
	warmup := 0
	if r.Scenario == "steady" {
		warmup = r.Warmup
	}
	out := make([]Sample, 0, r.Samples+warmup)
	for i := 0; i < r.Samples+warmup; i++ {
		instance, err := factory()
		if err != nil {
			return nil, err
		}
		if instance.Invoke == nil || instance.Close == nil || (p.Profile == "memory" && instance.BeginMemory == nil) {
			if instance.Close != nil {
				instance.Close()
			}
			return nil, fmt.Errorf("invalid trap factory")
		}
		var finish func() []Observation
		if p.Profile == "memory" {
			finish = instance.BeginMemory()
			if finish == nil {
				instance.Close()
				return nil, fmt.Errorf("missing trap memory snapshot")
			}
		}
		start := time.Now()
		callErr := instance.Invoke()
		elapsed := time.Since(start).Nanoseconds()
		var observations []Observation
		if finish != nil {
			observations = finish()
		}
		result := classify(callErr)
		instance.Close()
		if err := VerifyTrap(p.Workload, result); err != nil {
			return nil, fmt.Errorf("%w (invocation error: %v)", err, callErr)
		}
		out = append(out, Sample{Index: i, Warmup: i < warmup, ElapsedNS: elapsed, Operations: 1, SampleType: "individual_operation", Verified: true, TrapResult: result, Observations: observations})
	}
	return out, nil
}
