package analysis

import (
	"math"
	"sort"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

// ScalingMarginal is a finite difference between adjacent declared sizes,
// not a derivative, allocation volume, or a linear extrapolation. Pairing uses
// independent randomized blocks; each endpoint first reduces its inner samples
// to one launch median. Negative differences are retained, not clamped away.
type ScalingMarginal struct {
	FromWorkload string   `json:"from_workload"`
	ToWorkload   string   `json:"to_workload"`
	FromSize     int      `json:"from_size"`
	ToSize       int      `json:"to_size"`
	PairedBlocks int      `json:"paired_blocks"`
	Median       *float64 `json:"median_per_added_unit"`
	Low          *float64 `json:"ci95_low"`
	High         *float64 `json:"ci95_high"`
	Status       string   `json:"status"`
}

func scalingMarginals(c ScalingCurve, values map[string]map[string][]float64, trials []experiment.Trial) []ScalingMarginal {
	byBlock := map[string]map[int]float64{}
	counts := map[string]map[int]int{}
	for _, t := range trials {
		if t.Block < 0 || t.Runtime != c.Runtime || t.Scenario != c.Scenario || t.Profile != c.Profile {
			continue
		}
		if counts[t.Workload] == nil {
			counts[t.Workload] = map[int]int{}
		}
		counts[t.Workload][t.Block]++
		v := values[t.Workload][t.ID]
		if t.Status != "ok" || len(v) == 0 {
			continue
		}
		if byBlock[t.Workload] == nil {
			byBlock[t.Workload] = map[int]float64{}
		}
		byBlock[t.Workload][t.Block] = median(v)
	}
	var out []ScalingMarginal
	for i := 1; i < len(c.Points); i++ {
		a, b := c.Points[i-1], c.Points[i]
		m := ScalingMarginal{FromWorkload: a.Workload, ToWorkload: b.Workload, FromSize: a.Size, ToSize: b.Size, Status: "no_common_blocks"}
		if b.Size <= a.Size {
			m.Status = "ambiguous_input_size"
			out = append(out, m)
			continue
		}
		duplicate := false
		for _, id := range []string{a.Workload, b.Workload} {
			for _, n := range counts[id] {
				if n != 1 {
					duplicate = true
				}
			}
		}
		if duplicate {
			m.Status = "duplicate_trial_cell"
			out = append(out, m)
			continue
		}
		var blocks []int
		for block := range byBlock[a.Workload] {
			if _, ok := byBlock[b.Workload][block]; ok {
				blocks = append(blocks, block)
			}
		}
		sort.Ints(blocks)
		var deltas []float64
		invalid := false
		for _, block := range blocks {
			d := (byBlock[b.Workload][block] - byBlock[a.Workload][block]) / float64(b.Size-a.Size)
			if math.IsNaN(d) || math.IsInf(d, 0) {
				invalid = true
				break
			}
			deltas = append(deltas, d)
		}
		m.PairedBlocks = len(blocks)
		if invalid {
			m.Status = "numeric_range_exceeded"
		} else if len(deltas) > 0 {
			m.Median = protocol.Value(median(deltas))
			m.Status = "insufficient_blocks"
			if len(deltas) >= 3 {
				lo, hi := interval(deltas)
				m.Low, m.High = &lo, &hi
				m.Status = "available"
			}
		}
		out = append(out, m)
	}
	return out
}
