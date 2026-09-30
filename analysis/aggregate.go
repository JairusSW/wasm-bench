package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

const AggregateVersion = "fixed-set-paired-geomean-v1"

type AggregateMember struct {
	ID             string `json:"id"`
	ContractSHA256 string `json:"contract_sha256"`
}
type AggregateCategory struct {
	ID        string            `json:"id"`
	Weight    float64           `json:"weight"`
	Workloads []AggregateMember `json:"workloads"`
}
type AggregateSet struct {
	Schema     int                 `json:"schema"`
	ID         string              `json:"id"`
	Scenario   string              `json:"scenario"`
	Categories []AggregateCategory `json:"categories"`
}
type AggregateCoverage struct {
	Workload           string         `json:"workload"`
	Category           string         `json:"category"`
	Status             string         `json:"status"`
	BaselineOutcomes   map[string]int `json:"baseline_outcomes"`
	CandidateOutcomes  map[string]int `json:"candidate_outcomes"`
	PairedBlocks       []int          `json:"paired_blocks"`
	BaselineStability  string         `json:"baseline_stability"`
	CandidateStability string         `json:"candidate_stability"`
}
type AggregateResult struct {
	Category string   `json:"category"`
	Weight   float64  `json:"weight"`
	Required int      `json:"required_workloads"`
	Covered  int      `json:"covered_workloads"`
	Blocks   []int    `json:"complete_blocks"`
	Status   string   `json:"status"`
	Ratio    *float64 `json:"candidate_over_baseline"`
	Low      *float64 `json:"ci95_low"`
	High     *float64 `json:"ci95_high"`
}
type AggregateReport struct {
	Version        string              `json:"analysis_version"`
	Run            string              `json:"run"`
	LockSHA256     string              `json:"lock_sha256"`
	SetSHA256      string              `json:"set_sha256"`
	Set            AggregateSet        `json:"set"`
	Baseline       experiment.Runtime  `json:"baseline_configuration"`
	Candidate      experiment.Runtime  `json:"candidate_configuration"`
	PlannedBlocks  int                 `json:"planned_blocks"`
	Method         string              `json:"method"`
	Interpretation string              `json:"interpretation"`
	Coverage       []AggregateCoverage `json:"coverage"`
	Categories     []AggregateResult   `json:"categories"`
	Overall        AggregateResult     `json:"overall"`
}

// Artifact location is transport, not workload identity. Everything else,
// including input, oracle, reset, provenance and work units, remains pinned.
func aggregateContract(w protocol.Workload) string {
	w.Artifact = ""
	b, _ := json.Marshal(w)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// NewAggregateSet emits an explicit, editable proposal, never a hidden default
// score. Every category initially has weight 1 and members share that weight.
func NewAggregateSet(b experiment.Bundle, id, scenario string) (AggregateSet, error) {
	set := AggregateSet{Schema: 1, ID: id, Scenario: scenario}
	groups := map[string][]AggregateMember{}
	for _, w := range b.Manifest.Lock.Workloads {
		category := w.Family
		if category == "" {
			category = "uncategorized"
		}
		groups[category] = append(groups[category], AggregateMember{w.ID, aggregateContract(w)})
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		sort.Slice(groups[k], func(i, j int) bool { return groups[k][i].ID < groups[k][j].ID })
		set.Categories = append(set.Categories, AggregateCategory{k, 1, groups[k]})
	}
	return set, validateAggregateSet(set)
}

func validateAggregateSet(s AggregateSet) error {
	if s.Schema != 1 || s.ID == "" || !slices.Contains([]string{"engine-init", "compile", "instantiate", "app-init", "first-call", "steady", "cold-process", "aot-produce", "aot-load", "teardown"}, s.Scenario) || len(s.Categories) == 0 {
		return fmt.Errorf("invalid aggregate schema, id, scenario or empty categories")
	}
	cats, members := map[string]bool{}, map[string]bool{}
	var total float64
	for _, c := range s.Categories {
		if c.ID == "" || cats[c.ID] || c.Weight <= 0 || math.IsNaN(c.Weight) || math.IsInf(c.Weight, 0) || len(c.Workloads) == 0 {
			return fmt.Errorf("invalid or duplicate aggregate category %q", c.ID)
		}
		cats[c.ID] = true
		total += c.Weight
		for _, w := range c.Workloads {
			h, e := hex.DecodeString(w.ContractSHA256)
			if w.ID == "" || members[w.ID] || e != nil || len(h) != 32 || w.ContractSHA256 != hex.EncodeToString(h) {
				return fmt.Errorf("invalid or duplicate aggregate member %q", w.ID)
			}
			members[w.ID] = true
		}
	}
	if math.IsInf(total, 0) {
		return fmt.Errorf("aggregate weights overflow")
	}
	for _, c := range s.Categories {
		if c.Weight/total/float64(len(c.Workloads)) == 0 {
			return fmt.Errorf("aggregate member weight underflows")
		}
	}
	return nil
}

func Aggregate(b experiment.Bundle, set AggregateSet, baseline, candidate string) (AggregateReport, error) {
	b = hostEligibleBundle(b)
	out := AggregateReport{Version: AggregateVersion, Run: b.Manifest.ID, LockSHA256: b.Manifest.LockSHA256, Set: set, PlannedBlocks: b.Manifest.Lock.Options.Launches, Coverage: []AggregateCoverage{}, Categories: []AggregateResult{}, Method: "Within each complete block: weighted geometric mean of candidate/baseline launch-median ratios; equal member weights within category; median across block aggregates; 4000-resample percentile bootstrap over complete blocks", Interpretation: "Fixed declared workload set; no successful-subset score or category reweighting. Missing or changed contracts withhold the affected aggregate. Incomplete blocks are listed by omission from complete_blocks and remain in coverage outcomes. Intervals require at least three complete blocks. Descriptive local comparison, not official publication, causal attribution, or a universal runtime score. Workload sets and weights must be fixed before interpreting results; generating a proposal from a run does not prove preregistration."}
	if err := validateAggregateSet(set); err != nil {
		return out, err
	}
	if b.Manifest.Kind != "measurement" || b.Manifest.Lock.Options.Profile != "timing" || b.Manifest.Lock.Options.Check || b.Manifest.Lock.Options.PhaseBarriers {
		return out, fmt.Errorf("aggregate requires an uninstrumented timing measurement")
	}
	if baseline == candidate || baseline == "" || candidate == "" {
		return out, fmt.Errorf("two distinct runtime configurations required")
	}
	found := 0
	runtimeIDs := map[string]bool{}
	for _, r := range b.Manifest.Lock.Runtimes {
		if runtimeIDs[r.ID] {
			return out, fmt.Errorf("duplicate runtime configuration %q", r.ID)
		}
		runtimeIDs[r.ID] = true
		if r.ID == baseline {
			out.Baseline = r
			found++
		}
		if r.ID == candidate {
			out.Candidate = r
			found++
		}
	}
	if found != 2 {
		return out, fmt.Errorf("runtime configuration absent or duplicated")
	}
	if !slices.Contains(b.Manifest.Lock.Options.Scenarios, set.Scenario) || out.PlannedBlocks < 1 {
		return out, fmt.Errorf("scenario absent or invalid launch budget")
	}
	encoded, _ := json.Marshal(set)
	digest := sha256.Sum256(encoded)
	out.SetSHA256 = hex.EncodeToString(digest[:])
	workloads := map[string]protocol.Workload{}
	for _, w := range b.Manifest.Lock.Workloads {
		if _, ok := workloads[w.ID]; ok {
			return out, fmt.Errorf("duplicate workload %q", w.ID)
		}
		workloads[w.ID] = w
	}
	// Duplicate cells must not silently overwrite one another in Summarize.
	seen := map[string]bool{}
	for _, t := range b.Trials {
		if t.Block < 0 || t.Scenario != set.Scenario || (t.Runtime != baseline && t.Runtime != candidate) {
			continue
		}
		key := fmt.Sprintf("%s\x00%s\x00%d", t.Runtime, t.Workload, t.Block)
		if seen[key] || t.Block >= out.PlannedBlocks || t.Profile != "timing" {
			return out, fmt.Errorf("duplicate or incompatible aggregate trial %s", t.ID)
		}
		seen[key] = true
	}
	summaries := map[string]Summary{}
	for _, s := range Summarize(b) {
		if s.Profile == "timing" && s.Scenario == set.Scenario {
			summaries[s.Runtime+"\x00"+s.Workload] = s
		}
	}
	logs := map[string]map[int]float64{}
	for _, c := range set.Categories {
		for _, m := range c.Workloads {
			a, z := summaries[baseline+"\x00"+m.ID], summaries[candidate+"\x00"+m.ID]
			outcomes := func(s Summary) map[string]int {
				v := map[string]int{}
				for k, n := range s.Failures {
					v[k] = n
				}
				if missing := out.PlannedBlocks - s.Attempted; missing > 0 {
					v["not_attempted"] = missing
				}
				return v
			}
			row := AggregateCoverage{Workload: m.ID, Category: c.ID, Status: "no_positive_pairs", BaselineOutcomes: outcomes(a), CandidateOutcomes: outcomes(z), PairedBlocks: []int{}, BaselineStability: a.Stability, CandidateStability: z.Stability}
			w, ok := workloads[m.ID]
			switch {
			case !ok:
				row.Status = "missing_workload"
			case aggregateContract(w) != m.ContractSHA256:
				row.Status = "contract_mismatch"
			default:
				logs[m.ID] = map[int]float64{}
				for block := 0; block < out.PlannedBlocks; block++ {
					av, aok := a.LaunchMedians[block]
					cv, cok := z.LaunchMedians[block]
					if aok && cok && av > 0 && cv > 0 && !math.IsInf(av, 0) && !math.IsInf(cv, 0) {
						logs[m.ID][block] = math.Log(cv) - math.Log(av)
						row.PairedBlocks = append(row.PairedBlocks, block)
					}
				}
				if len(row.PairedBlocks) > 0 {
					row.Status = "paired"
				}
			}
			out.Coverage = append(out.Coverage, row)
		}
	}
	result := func(id string, weight float64, categories []AggregateCategory) AggregateResult {
		r := AggregateResult{Category: id, Weight: weight, Blocks: []int{}, Status: "incomplete_coverage"}
		total := 0.0
		for _, c := range categories {
			total += c.Weight
			for _, w := range c.Workloads {
				r.Required++
				if len(logs[w.ID]) > 0 {
					r.Covered++
				}
			}
		}
		if r.Required != r.Covered {
			return r
		}
		var values []float64
		for block := 0; block < out.PlannedBlocks; block++ {
			v, complete := 0.0, true
			for _, c := range categories {
				for _, w := range c.Workloads {
					x, ok := logs[w.ID][block]
					if !ok {
						complete = false
					}
					v += x * (c.Weight / total) / float64(len(c.Workloads))
				}
			}
			if complete {
				ratio := math.Exp(v)
				if math.IsInf(ratio, 0) || ratio == 0 {
					r.Status = "numeric_range_exceeded"
					return r
				}
				values = append(values, ratio)
				r.Blocks = append(r.Blocks, block)
			}
		}
		r.Status = "no_common_complete_blocks"
		if len(values) == 0 {
			return r
		}
		r.Ratio = protocol.Value(median(values))
		r.Status = "insufficient_blocks"
		if len(values) >= 3 {
			lo, hi := interval(values)
			r.Low, r.High = &lo, &hi
			r.Status = "available"
		}
		return r
	}
	for _, c := range set.Categories {
		out.Categories = append(out.Categories, result(c.ID, c.Weight, []AggregateCategory{c}))
	}
	out.Overall = result("all", 1, set.Categories)
	return out, nil
}
