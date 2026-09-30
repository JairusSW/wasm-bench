package analysis

import (
	"math/big"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const ThroughputVersion = "timed-work-launch-rates-v2"

// ThroughputLaunch retains exact decimal totals before conversion to a display
// rate. Null totals mean the launch was excluded, never measured zero.
type ThroughputLaunch struct {
	Trial       string   `json:"trial"`
	Block       int      `json:"block"`
	TrialStatus string   `json:"trial_status"`
	TrialReason string   `json:"trial_reason,omitempty"`
	Status      string   `json:"status"`
	Operations  *string  `json:"measured_operations_decimal"`
	WorkUnits   *string  `json:"measured_work_units_decimal"`
	ElapsedNS   *string  `json:"measured_elapsed_ns_decimal"`
	Rate        *float64 `json:"work_units_per_second"`
}

// ThroughputSummary is useful work divided by timed-region duration, not a
// sustained service rate. Setup, verification and inter-sample gaps are excluded.
type ThroughputSummary struct {
	Runtime            string             `json:"runtime"`
	Workload           string             `json:"workload"`
	Scenario           string             `json:"scenario"`
	Profile            string             `json:"profile"`
	WorkUnit           string             `json:"work_unit"`
	UnitsPerInvocation string             `json:"units_per_invocation_decimal"`
	Status             string             `json:"status"`
	Reason             string             `json:"reason"`
	Attempted          int                `json:"attempted_launches"`
	Launches           int                `json:"eligible_launches"`
	Excluded           map[string]int     `json:"excluded_launches"`
	Median             *float64           `json:"median_work_units_per_second"`
	Low                *float64           `json:"ci95_low"`
	High               *float64           `json:"ci95_high"`
	Rates              map[int]float64    `json:"launch_rates"`
	Evidence           []ThroughputLaunch `json:"launch_evidence"`
}

func Throughput(b experiment.Bundle) []ThroughputSummary {
	policy := HeadlineLatencyPolicy(b.Manifest)
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		workloads[w.ID] = w
	}
	type group struct {
		summary ThroughputSummary
		blocks  map[int][]experiment.Trial
	}
	groups := map[string]*group{}
	for _, t := range b.Trials {
		if t.Block < 0 {
			continue
		}
		key := t.Runtime + "\x00" + t.Workload + "\x00" + t.Scenario + "\x00" + t.Profile
		g := groups[key]
		if g == nil {
			w := workloads[t.Workload]
			p := policy.ForProfile(t.Profile)
			g = &group{summary: ThroughputSummary{Runtime: t.Runtime, Workload: t.Workload, Scenario: t.Scenario, Profile: t.Profile, WorkUnit: w.WorkUnit, UnitsPerInvocation: new(big.Int).SetUint64(w.Units).String(), Status: p.Status, Reason: p.Reason, Excluded: map[string]int{}, Rates: map[int]float64{}}, blocks: map[int][]experiment.Trial{}}
			if p.Status == "timing_pass" {
				switch {
				case t.Scenario != "steady" && t.Scenario != "first-call" && t.Scenario != "trajectory" && t.Scenario != "sustained":
					g.summary.Status, g.summary.Reason = "not_applicable", "Only workload-call timing regions define useful-work throughput; setup and teardown are not workload execution."
				case w.WorkUnit == "" || w.Units == 0:
					g.summary.Status, g.summary.Reason = "missing_work_contract", "A nonempty work unit and positive units per invocation are required."
				}
			}
			groups[key] = g
		}
		g.summary.Attempted++
		g.blocks[t.Block] = append(g.blocks[t.Block], t)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]ThroughputSummary, 0, len(keys))
	for _, key := range keys {
		g := groups[key]
		s := &g.summary
		blocks := make([]int, 0, len(g.blocks))
		for block := range g.blocks {
			blocks = append(blocks, block)
		}
		sort.Ints(blocks)
		var rates []float64
		for _, block := range blocks {
			trials := g.blocks[block]
			if s.Status != "timing_pass" {
				for _, t := range trials {
					s.Evidence = append(s.Evidence, excludedThroughput(t, s.Status))
					s.Excluded[s.Status]++
				}
				continue
			}
			if len(trials) != 1 {
				s.Excluded["duplicate_block"] += len(trials)
				for _, t := range trials {
					s.Evidence = append(s.Evidence, excludedThroughput(t, "duplicate_block"))
				}
				continue
			}
			evidence := timedWorkEvidence(trials[0], workloads[s.Workload].Units)
			s.Evidence = append(s.Evidence, evidence)
			if evidence.Status != "available" {
				s.Excluded[evidence.Status]++
				continue
			}
			rate := *evidence.Rate
			s.Rates[block] = rate
			rates = append(rates, rate)
		}
		if s.Status != "timing_pass" {
			out = append(out, *s)
			continue
		}
		s.Launches = len(rates)
		s.Status, s.Reason = "unavailable", "No eligible independent launch has positive measured duration."
		if len(rates) > 0 {
			s.Median = protocol.Value(median(rates))
			s.Status, s.Reason = "insufficient_launches", "Median of independent-launch work rates; fewer than three launches, so interval withheld. Excludes setup, verification and inter-sample gaps; not sustained service throughput."
		}
		if len(rates) >= 3 {
			lo, hi := interval(rates)
			s.Low, s.High = &lo, &hi
			s.Status, s.Reason = "available", "Median and 95% percentile bootstrap over independent-launch work rates. Excludes setup, verification and inter-sample gaps; not sustained service throughput."
		}
		out = append(out, *s)
	}
	return out
}

func excludedThroughput(t experiment.Trial, status string) ThroughputLaunch {
	return ThroughputLaunch{Trial: t.ID, Block: t.Block, TrialStatus: t.Status, TrialReason: t.Reason, Status: status}
}

func timedWorkEvidence(t experiment.Trial, units uint64) ThroughputLaunch {
	if t.Status != "ok" {
		return excludedThroughput(t, t.Status)
	}
	operations, elapsed := new(big.Int), new(big.Int)
	count := 0
	for _, s := range t.Samples {
		if s.Warmup {
			continue
		}
		if !s.Verified || s.Operations <= 0 || s.ElapsedNS < 0 {
			return excludedThroughput(t, "invalid_sample")
		}
		if s.SampleType != "batch_average" && s.SampleType != "individual_operation" && s.SampleType != "sequence_call_sum" {
			return excludedThroughput(t, "unknown_sample_type")
		}
		operations.Add(operations, big.NewInt(int64(s.Operations)))
		elapsed.Add(elapsed, big.NewInt(s.ElapsedNS))
		count++
	}
	if count == 0 {
		return excludedThroughput(t, "no_measured_samples")
	}
	if elapsed.Sign() == 0 {
		return excludedThroughput(t, "unresolved_timer_resolution")
	}
	// Exact integer sums prevent overflow before the final display conversion.
	opDecimal, elapsedDecimal := operations.String(), elapsed.String()
	operations.Mul(operations, new(big.Int).SetUint64(units))
	workDecimal := operations.String()
	operations.Mul(operations, big.NewInt(1_000_000_000))
	rate, _ := new(big.Rat).SetFrac(operations, elapsed).Float64()
	e := excludedThroughput(t, "available")
	e.Operations, e.WorkUnits, e.ElapsedNS, e.Rate = &opDecimal, &workDecimal, &elapsedDecimal, &rate
	return e
}
