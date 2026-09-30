package publish

import (
	"github.com/wasmbench/wasmbench/experiment"
	"strconv"
)

const CounterDisplayVersion = "raw-counter-decimal-display-v1"

// Shadow only the uint64 fields with decimal strings for browser-safe JSON.
// Raw bundle and Parquet types are unchanged; this is a display projection.
type CounterDisplayRow struct {
	CounterRow
	RawCount    *string `json:"RawCount"`
	EventConfig *string `json:"EventConfig"`
	EnabledNS   *string `json:"EnabledNS"`
	RunningNS   *string `json:"RunningNS"`
}

func decimalCounter(v *uint64) *string {
	if v == nil {
		return nil
	}
	s := strconv.FormatUint(*v, 10)
	return &s
}

func CounterDisplay(b experiment.Bundle) []CounterDisplayRow {
	var rows []CounterDisplayRow
	_ = eachCounterRow(b, func(r CounterRow) error {
		rows = append(rows, CounterDisplayRow{CounterRow: r, RawCount: decimalCounter(r.RawCount), EventConfig: decimalCounter(r.EventConfig), EnabledNS: decimalCounter(r.EnabledNS), RunningNS: decimalCounter(r.RunningNS)})
		return nil
	})
	return rows
}
