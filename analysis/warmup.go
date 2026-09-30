package analysis

import (
	"math"

	"github.com/wasmbench/wasmbench/experiment"
)

const WarmupVersion = "post-warmup-thirds-v1"

// WarmupDiagnostic is a descriptive screen, not a statistical convergence test.
// It never changes the samples used for performance summaries.
type WarmupDiagnostic struct {
	Version              string    `json:"version"`
	Trial                string    `json:"trial"`
	Block                int       `json:"block"`
	Status               string    `json:"status"`
	Reason               string    `json:"reason"`
	WarmupSamples        int       `json:"warmup_samples"`
	MeasuredSamples      int       `json:"measured_samples"`
	WindowMedians        []float64 `json:"window_medians_ns_per_operation,omitempty"`
	RelativeMedianSpread *float64  `json:"relative_median_spread"`
	RelativeMAD          *float64  `json:"relative_mad"`
}

func diagnoseWarmup(t experiment.Trial) WarmupDiagnostic {
	d := WarmupDiagnostic{Version: WarmupVersion, Trial: t.ID, Block: t.Block, Status: "not_applicable", Reason: "requires a timing steady or trajectory trial"}
	if t.Profile != "timing" || (t.Scenario != "steady" && t.Scenario != "trajectory" && t.Scenario != "sustained") {
		return d
	}
	if t.Status != "ok" {
		d.Status, d.Reason = "unavailable", "trial did not succeed"
		return d
	}
	var values []float64
	for i, s := range t.Samples {
		if !s.Verified || s.ElapsedNS < 0 || s.Operations <= 0 || (i > 0 && s.Index != t.Samples[i-1].Index+1) {
			d.Status, d.Reason = "unavailable", "invalid, unverified or nonconsecutive samples"
			return d
		}
		if i > 0 && (s.Operations != t.Samples[0].Operations || s.SampleType != t.Samples[0].SampleType) {
			d.Status, d.Reason = "unavailable", "sample boundaries change within the sequence"
			return d
		}
		if s.Warmup {
			if len(values) > 0 {
				d.Status, d.Reason = "unavailable", "warmup is not a prefix"
				return d
			}
			d.WarmupSamples++
		} else {
			values = append(values, float64(s.ElapsedNS)/float64(s.Operations))
		}
	}
	d.MeasuredSamples = len(values)
	if len(values) < 15 {
		d.Status, d.Reason = "insufficient_samples", "requires at least 15 post-warmup observations (five per third)"
		return d
	}
	center := median(values)
	if center == 0 {
		d.Status, d.Reason = "unresolved_timer_resolution", "zero median prevents relative diagnostics"
		return d
	}
	for i := 0; i < 3; i++ {
		d.WindowMedians = append(d.WindowMedians, median(values[i*len(values)/3:(i+1)*len(values)/3]))
	}
	low, high := d.WindowMedians[0], d.WindowMedians[0]
	for _, v := range d.WindowMedians {
		low = math.Min(low, v)
		high = math.Max(high, v)
	}
	spread := (high - low) / center
	deviations := make([]float64, len(values))
	for i, v := range values {
		deviations[i] = math.Abs(v - center)
	}
	mad := median(deviations) / center
	d.RelativeMedianSpread, d.RelativeMAD = &spread, &mad
	d.Status, d.Reason = "no_drift_detected", "three chronological thirds have median spread <=10% and relative MAD <=10%; this does not establish convergence or a steady tier"
	if mad > 0.1 {
		d.Status, d.Reason = "high_variability", "post-warmup median absolute deviation exceeds 10% of the median"
	}
	if spread > 0.1 {
		d.Status, d.Reason = "drift_detected", "chronological-third median spread exceeds 10% of the full post-warmup median; non-convergence suspected, not proven"
	}
	return d
}
