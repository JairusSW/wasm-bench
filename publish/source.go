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
	"github.com/wasmbench/wasmbench/sourcebuild"
)

//go:embed source.html
var sourceAssets embed.FS

func SourceReport(bundle, out string) error {
	if err := reportOutsideInputs([]string{bundle}, out); err != nil {
		return err
	}
	b, err := sourcebuild.VerifyBenchmark(bundle)
	if err != nil {
		return err
	}
	digest, err := experiment.DigestFile(filepath.Join(bundle, "checksums.json"))
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
	if err = os.CopyFS(raw, os.DirFS(bundle)); err != nil {
		return err
	}
	if _, err = sourcebuild.VerifyBenchmark(raw); err != nil {
		return err
	}
	copied, err := experiment.DigestFile(filepath.Join(raw, "checksums.json"))
	if err != nil {
		return err
	}
	if copied != digest {
		return fmt.Errorf("source evidence changed during report copy")
	}
	if err = ExportSourceTables(raw, out); err != nil {
		return err
	}
	d := struct {
		Report          analysis.SourceBuildReport `json:"report"`
		ChecksumsSHA256 string                     `json:"source_checksums_sha256"`
	}{analysis.SummarizeSourceBuilds(b), digest}
	if err = experiment.WriteJSON(filepath.Join(out, "data.json"), d); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(sourceAssets, "source.html")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = t.Execute(f, struct{ Data template.JS }{template.JS(data)})
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return sealFamilyReport(out, "source")
}
