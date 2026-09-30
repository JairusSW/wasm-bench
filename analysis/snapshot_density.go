package analysis

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const SnapshotDensityAnalysisVersion = "snapshot-density-launch-medians-v2"

// SnapshotDensity analyzes only complete, validated, memory-profile groups.
// PSS/private totals include source, template and every restored child. These
// are sums of non-atomic boundary readings, not physical peaks or summed RSS.
// Inner groups reduce to a launch median before launch-level uncertainty and
// block-paired finite differences are calculated.
func SnapshotDensity(b experiment.Bundle) ([]ScalingCurve, error) {
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if w.SnapshotDensity != nil {
			if err := protocol.ValidateSnapshotDensityWorkload(w); err != nil {
				return nil, err
			}
			workloads[w.ID] = w
		}
	}
	type accumulated struct {
		curve  ScalingCurve
		values map[string]map[string][]float64
	}
	groups := map[string]*accumulated{}
	// Validate before host eligibility masks execution outcomes.
	for _, t := range b.Trials {
		w, ok := workloads[t.Workload]
		if !ok || t.Block < 0 || t.Status != "ok" {
			continue
		}
		if t.Profile != "memory" || t.Scenario != protocol.SnapshotDensityScenario || t.SnapshotDensity == nil {
			return nil, fmt.Errorf("density trial %s has no complete memory evidence", t.ID)
		}
		r := protocol.RunRequest{Scenario: t.Scenario, Samples: b.Manifest.Lock.Options.Samples, Operations: b.Manifest.Lock.Options.Operations, Warmup: b.Manifest.Lock.Options.Warmup, PhaseBarriers: b.Manifest.Lock.Options.PhaseBarriers}
		backend := ""
		for _, runtime := range b.Manifest.Lock.Runtimes {
			if runtime.ID == t.Runtime && runtime.Description != nil {
				backend = runtime.Description.Backend
			}
		}
		if err := experiment.ValidateSnapshotDensityTrial(w, r, backend, *t.SnapshotDensity); err != nil {
			return nil, fmt.Errorf("density trial %s: %w", t.ID, err)
		}
	}
	b = hostEligibleBundle(b)
	for _, t := range b.Trials {
		w, ok := workloads[t.Workload]
		if !ok || t.Block < 0 || t.Profile != "memory" || t.Scenario != protocol.SnapshotDensityScenario {
			continue
		}
		for _, stage := range []string{"idle", "touched", "executed", "provision"} {
			metrics := []string{"process_group.boundary_pss_sum", "process_group.boundary_private_sum"}
			if stage == "provision" {
				metrics = []string{"snapshot.provision.elapsed"}
			}
			if stage == "touched" {
				metrics = append(metrics, "snapshot.child_touch.mean_elapsed")
			}
			if stage == "executed" {
				metrics = append(metrics, "snapshot.child_execution.mean_elapsed")
			}
			for _, metric := range metrics {
				key := t.Runtime + "\x00" + stage + "\x00" + metric
				g := groups[key]
				if g == nil {
					g = &accumulated{curve: ScalingCurve{Runtime: t.Runtime, Generator: w.Generator, Dimension: "instances", Scenario: t.Scenario, Profile: "memory", Measurement: protocol.Observation{Metric: metric, DefinitionVersion: 1, Unit: "bytes", Scope: "held_source_template_and_restored_process_group", Phase: stage, Collector: "linux_procfs_smaps_rollup", Quality: "non_atomic_boundary_sum", Profile: "memory", Denominator: "restored_process_group"}, FitStatus: "not_fitted_non_atomic_boundary_accounting"}, values: map[string]map[string][]float64{}}
					groups[key] = g
					g.curve.Measurement.CollectorVersion = collectors.SnapshotProcessCollectorVersion
					if metric == "snapshot.provision.elapsed" || metric == "snapshot.child_touch.mean_elapsed" || metric == "snapshot.child_execution.mean_elapsed" {
						g.curve.Measurement.Unit = "ns"
						g.curve.Measurement.Quality = "diagnostic_memory_pass"
						g.curve.Measurement.Collector = "adapter_process_monotonic_clock"
						g.curve.Measurement.CollectorVersion = protocol.SnapshotDensityWorkerVersion
						g.curve.Measurement.Scope = "restored_child_local_region_mean"
						g.curve.Measurement.Denominator = "restored_child"
						g.curve.FitStatus = "not_fitted_instrumented_diagnostic_clocks"
						if stage == "provision" {
							g.curve.Measurement.Scope = "source_restore_request_to_all_live_ready_identity_frames"
							g.curve.Measurement.Denominator = "restored_process_group"
						}
					}
				}
				if t.Status != "ok" {
					continue
				}
				for _, sample := range t.SnapshotDensity.Groups {
					if g.curve.Measurement.Unit == "ns" {
						value, err := densityDiagnosticTime(sample.Proof, metric)
						if err != nil {
							return nil, err
						}
						if g.values[w.ID] == nil {
							g.values[w.ID] = map[string][]float64{}
						}
						g.values[w.ID][t.ID] = append(g.values[w.ID][t.ID], value)
						continue
					}
					for _, record := range sample.Records {
						if record.Boundary.Stage != stage {
							continue
						}
						value, available, err := densityBoundarySum(record, metric)
						if err != nil {
							return nil, err
						}
						if !available {
							continue
						}
						if g.values[w.ID] == nil {
							g.values[w.ID] = map[string][]float64{}
						}
						g.values[w.ID][t.ID] = append(g.values[w.ID][t.ID], value)
					}
				}
			}
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []ScalingCurve
	for _, key := range keys {
		g := groups[key]
		c := g.curve
		for _, w := range workloads {
			p := ScalingPoint{Workload: w.ID, Size: w.Size, Status: "unavailable", Outcomes: map[string]int{}}
			var launchValues []float64
			for _, t := range b.Trials {
				if t.Block < 0 || t.Runtime != c.Runtime || t.Workload != w.ID || t.Scenario != c.Scenario || t.Profile != c.Profile {
					continue
				}
				p.Attempted++
				outcome := t.Status
				v := g.values[w.ID][t.ID]
				// A partially readable launch must not select its convenient groups.
				if t.Status == "ok" && len(v) != b.Manifest.Lock.Options.Samples {
					outcome = "incomplete_smaps_coverage"
				} else if t.Status == "ok" {
					launchValues = append(launchValues, median(v))
				}
				p.Outcomes[outcome]++
				if outcome != "ok" {
					delete(g.values[w.ID], t.ID)
				}
			}
			p.Launches = len(launchValues)
			if p.Launches > 0 {
				p.Median = protocol.Value(median(launchValues))
				p.Status = "insufficient_launches"
			}
			if p.Launches >= 3 {
				lo, hi := interval(launchValues)
				p.Low = &lo
				p.High = &hi
				p.Status = "available"
			}
			c.Points = append(c.Points, p)
		}
		sort.Slice(c.Points, func(i, j int) bool {
			if c.Points[i].Size == c.Points[j].Size {
				return c.Points[i].Workload < c.Points[j].Workload
			}
			return c.Points[i].Size < c.Points[j].Size
		})
		c.Marginal = scalingMarginals(c, g.values, b.Trials)
		out = append(out, c)
	}
	return out, nil
}

// Child clocks are never added to imply elapsed group wall time. Provisioning
// uses the source clock; touch/execution are arithmetic means of child-local
// regions within a group, not independent replications or headline latency.
func densityDiagnosticTime(proof protocol.SnapshotDensityProof, metric string) (float64, error) {
	values := []*int64{proof.ProvisionElapsedNS}
	if metric != "snapshot.provision.elapsed" {
		values = nil
		for _, child := range proof.Children {
			if metric == "snapshot.child_touch.mean_elapsed" {
				values = append(values, child.TouchElapsedNS)
			} else if metric == "snapshot.child_execution.mean_elapsed" {
				values = append(values, child.ExecuteElapsedNS)
			} else {
				return 0, fmt.Errorf("unknown density clock metric")
			}
		}
	}
	if len(values) == 0 {
		return 0, fmt.Errorf("missing density clocks")
	}
	total := new(big.Int)
	for _, value := range values {
		if value == nil || *value < 0 || *value > 9007199254740991 {
			return 0, fmt.Errorf("invalid or inexact density clock")
		}
		total.Add(total, big.NewInt(*value))
	}
	mean := new(big.Rat).SetFrac(total, big.NewInt(int64(len(values))))
	value, _ := mean.Float64()
	return value, nil
}

func densityBoundarySum(record experiment.SnapshotDensityRecord, metric string) (float64, bool, error) {
	total := new(big.Int)
	for _, reading := range record.Readings {
		f, err := collectors.ValidateSnapshotProcessReading(reading, reading.Process, reading.ParentPID)
		if err != nil {
			return 0, false, err
		}
		value := f.PSSBytes
		if metric == "process_group.boundary_private_sum" {
			value = f.PrivateBytes
		}
		if value == nil {
			return 0, false, nil
		}
		total.Add(total, new(big.Int).SetUint64(*value))
	}
	// Reject rather than silently rounding exact byte sums in browser datasets.
	if total.BitLen() > 53 {
		return 0, false, fmt.Errorf("density boundary sum exceeds exact JSON numeric range")
	}
	return float64(total.Uint64()), true, nil
}
