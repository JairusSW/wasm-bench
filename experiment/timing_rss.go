package experiment

import (
	"fmt"
	"math"
)

// ValidateTimingPeakRSS binds the wait4 observation to the successful timing
// trial that collected it. A missing peak is not silently a memory measurement.
func ValidateTimingPeakRSS(b Bundle) error {
	if !b.Manifest.Lock.Options.TimingPeakRSS {
		return nil
	}
	if b.Manifest.Lock.Options.Profile != "timing" {
		return fmt.Errorf("timing peak RSS requires timing profile")
	}
	for _, trial := range b.Trials {
		if trial.Block < 0 || trial.Status != "ok" {
			continue
		}
		count := 0
		for _, o := range trial.Observations {
			if o.Metric != "process.peak_rss" {
				continue
			}
			count++
			if trial.Profile != "timing" || o.Profile != "timing" || o.DefinitionVersion != 1 || o.Unit != "bytes" || o.Scope != "adapter_process" || o.Phase != trial.Scenario+"/process_lifetime" || o.Collector != "wait4_rusage" || o.CollectorVersion != "1" || o.Quality != "kernel_accounted_peak" || o.Denominator != "process" {
				return fmt.Errorf("timing peak RSS provenance differs from trial %s", trial.ID)
			}
			if o.Status == "available" {
				if o.Value == nil || math.IsNaN(*o.Value) || math.IsInf(*o.Value, 0) || *o.Value < 0 {
					return fmt.Errorf("invalid timing peak RSS in %s", trial.ID)
				}
			} else if o.Status != "unavailable" || o.Value != nil || o.Reason == "" {
				return fmt.Errorf("invalid unavailable timing peak RSS in %s", trial.ID)
			}
		}
		if count != 1 {
			return fmt.Errorf("timing trial %s must contain exactly one peak RSS observation", trial.ID)
		}
	}
	return nil
}
