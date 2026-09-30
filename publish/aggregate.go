package publish

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

//go:embed aggregate.html
var aggregateAssets embed.FS

type aggregateDataset struct {
	Analysis              analysis.AggregateReport `json:"analysis"`
	SourceChecksumsSHA256 string                   `json:"source_checksums_sha256"`
}

func ExportAggregateSet(root, id, scenario, out string) error {
	if err := reportOutsideInputs([]string{root}, out); err != nil {
		return err
	}
	b, err := experiment.Load(root)
	if err != nil {
		return err
	}
	set, err := analysis.NewAggregateSet(b, id, scenario)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	return experiment.WriteJSON(out, set)
}

// AggregateReport copies and verifies the original evidence, then derives all
// displayed values from that copy. Existing reports and inputs are never edited.
func AggregateReport(root string, set analysis.AggregateSet, baseline, candidate, out string) error {
	if err := reportOutsideInputs([]string{root}, out); err != nil {
		return err
	}
	b, err := experiment.Load(root)
	if err != nil {
		return err
	}
	if _, err = analysis.Aggregate(b, set, baseline, candidate); err != nil {
		return err
	}
	receipt, err := experiment.DigestFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	raw := filepath.Join(out, "raw")
	if err = os.CopyFS(raw, os.DirFS(root)); err != nil {
		return err
	}
	b, err = experiment.Load(raw)
	if err != nil {
		return err
	}
	copied, err := experiment.DigestFile(filepath.Join(raw, "checksums.json"))
	if err != nil {
		return err
	}
	if copied != receipt {
		return fmt.Errorf("aggregate evidence changed while copying")
	}
	report, err := analysis.Aggregate(b, set, baseline, candidate)
	if err != nil {
		return err
	}
	data := aggregateDataset{report, receipt}
	for name, value := range map[string]any{"data.json": data, "set.json": set, "trials.json": b.Trials} {
		if err = experiment.WriteJSON(filepath.Join(out, name), value); err != nil {
			return err
		}
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return err
	}
	tpl, err := template.ParseFS(aggregateAssets, "aggregate.html")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	// encoding/json escapes HTML-sensitive characters before trusted insertion.
	err = tpl.Execute(f, struct{ Data template.JS }{template.JS(encoded)})
	closed := f.Close()
	if err != nil {
		return err
	}
	if closed != nil {
		return closed
	}
	return sealFamilyReport(out, "aggregate")
}
