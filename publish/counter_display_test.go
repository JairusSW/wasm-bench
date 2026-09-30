package publish

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/collectors"
	"github.com/wasmbench/wasmbench/experiment"
	"math"
	"testing"
)

func TestCounterDisplayExactDecimalJSON(t *testing.T) {
	max, zero := uint64(math.MaxUint64), uint64(0)
	b := experiment.Bundle{Trials: []experiment.Trial{{ID: "x", Profile: "counters", Status: "error", CounterPhases: []agent.CounterPhase{{Status: "partial", Readings: []collectors.PerfReading{{Count: &max, EnabledNS: &max, RunningNS: &zero, Event: collectors.PerfEvent{Config: math.MaxUint64}, Status: "coverage_changed"}, {Status: "permission_denied"}}}}}, {ID: "unsupported", Profile: "counters", Status: "unsupported"}}}
	rows := CounterDisplay(b)
	if len(rows) != 3 {
		t.Fatal(rows)
	}
	data, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	var parsed []map[string]any
	if err = json.Unmarshal(data, &parsed); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"RawCount", "EnabledNS", "EventConfig"} {
		if parsed[0][key] != "18446744073709551615" {
			t.Fatal(key, parsed[0])
		}
	}
	if parsed[0]["RunningNS"] != "0" || parsed[0]["ReadingStatus"] != "coverage_changed" || parsed[1]["RawCount"] != nil || parsed[2]["RowKind"] != "trial_outcome" {
		t.Fatal(parsed)
	}
	*rows[0].RawCount = "edited"
	if *b.Trials[0].CounterPhases[0].Readings[0].Count != max {
		t.Fatal("raw evidence mutated")
	}
}
