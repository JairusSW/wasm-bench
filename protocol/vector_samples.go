package protocol

import (
	"fmt"
	"slices"
	"time"
)

// VectorFactory returns a fresh initialized instance and the measured setup
// duration for compile/instantiate. Initialization itself is not timed.
// For phased instantiation, the factory owns before_instantiate/instantiated
// handshakes and API-window diagnostics, using the sequential sample index.
// RunVectorSamples emits instance_released only after every vector verifies.
type VectorFactory func(string) (VectorInstance, int64, func(), error)

func RunVectorSamples(p *Preparation, r *RunRequest, factory VectorFactory, diagnostics ...func() func() []Observation) ([]Sample, error) {
	if p == nil || r == nil || factory == nil {
		return nil, fmt.Errorf("missing vector preparation/request/factory")
	}
	if !slices.Contains([]string{"timing", "memory"}, p.Profile) || (r.PhaseBarriers && (p.Profile != "memory" || !slices.Contains([]string{"instantiate", "first-call", "teardown"}, r.Scenario))) || !slices.Contains([]string{"compile", "instantiate", "first-call", "steady", "teardown"}, r.Scenario) {
		return nil, fmt.Errorf("unsupported vector scenario/profile")
	}
	if r.Samples < 1 || r.Samples > 100000 || r.Warmup < 0 || r.Warmup > 100000 || r.Operations < 1 {
		return nil, fmt.Errorf("invalid vector batch")
	}
	prepared, err := PrepareWorkloadVectors(p.Workload)
	if err != nil {
		return nil, err
	}
	warmup := 0
	if r.Scenario == "steady" {
		warmup = r.Warmup
	}
	out := make([]Sample, 0, r.Samples+warmup)
	for i := -warmup; i < r.Samples; i++ {
		var finish func() []Observation
		if p.Profile == "memory" && len(diagnostics) > 0 && r.Scenario != "teardown" && !(r.PhaseBarriers && r.Scenario == "first-call") {
			finish = diagnostics[0]()
		}
		instance, elapsed, close, err := factory(r.Scenario)
		if err != nil {
			return nil, err
		}
		if instance.Invoke == nil || close == nil || (r.PhaseBarriers && instance.ReleaseBarrier == nil) {
			if close != nil {
				close()
			}
			return nil, fmt.Errorf("invalid vector instance factory")
		}
		invoke := instance.Invoke
		var callObservations []Observation
		calls := 0
		sampleType := "individual_operation"
		if r.Scenario == "first-call" || r.Scenario == "steady" {
			elapsed = 0
			sampleType = "sequence_call_sum"
			instance.Invoke = func(in, n, out uint32) error {
				if r.PhaseBarriers && calls == 0 {
					if err := instance.ReleaseBarrier(i, "before_first_call"); err != nil {
						return err
					}
					if len(diagnostics) > 0 {
						finish = diagnostics[0]()
					}
				}
				start := time.Now()
				err := invoke(in, n, out)
				elapsed += time.Since(start).Nanoseconds()
				calls++
				if err == nil && r.PhaseBarriers && calls == len(prepared.cases) {
					if finish != nil {
						callObservations = finish()
						finish = nil
					}
					err = instance.ReleaseBarrier(i, "first_call_returned")
				}
				return err
			}
		}
		err = prepared.Run(instance)
		var observations []Observation
		if err == nil && p.Profile == "memory" && instance.Snapshot != nil {
			observations = instance.Snapshot()
		}
		observations = append(observations, callObservations...)
		if err == nil && r.Scenario == "teardown" {
			if r.PhaseBarriers {
				if err := instance.ReleaseBarrier(i, "before_teardown"); err != nil {
					close()
					return nil, err
				}
			}
			if p.Profile == "memory" && len(diagnostics) > 0 {
				finish = diagnostics[0]()
			}
			start := time.Now()
			close()
			elapsed = time.Since(start).Nanoseconds()
		} else {
			close()
		}
		if err != nil {
			return nil, err
		}
		s := Sample{Index: i + warmup, Warmup: i < 0, ElapsedNS: elapsed, Operations: 1, SampleType: sampleType, Verified: true}
		s.Observations = observations
		if finish != nil {
			s.Observations = append(s.Observations, finish()...)
		}
		if r.PhaseBarriers {
			stage := "torn_down"
			if r.Scenario == "instantiate" {
				stage = "instance_released"
			} else if r.Scenario == "first-call" {
				stage = "first_call_released"
			}
			if err := instance.ReleaseBarrier(i, stage); err != nil {
				return nil, err
			}
		}
		out = append(out, s)
	}
	return out, nil
}

func PrepareWorkloadVectors(w Workload) (*PreparedVectors, error) {
	if w.Vectors == nil || w.Oracle.Kind != "exact_vectors" || w.Reset != "fresh_instance_per_sample" || w.Input != nil || w.Command != nil || len(w.Args) != 0 || len(w.Oracle.Memory) != 0 || len(w.Oracle.Expected) != 0 || w.Oracle.OutputPointerExport != "" || w.HostProfile != "" {
		return nil, fmt.Errorf("unsupported or ambiguous vector contract")
	}
	return PrepareVectors(*w.Vectors, w.VectorByteBudget)
}
