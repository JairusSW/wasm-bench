package publish

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/wasmbench/wasmbench/analysis"
	"github.com/wasmbench/wasmbench/experiment"
)

//go:embed history.html
var historyAssets embed.FS

// HistoryReport regenerates analysis from verified inputs; it never accepts an
// arbitrary precomputed JSON report as publication evidence.
func HistoryReport(paths []string, runtime, out string) error {
	if err := reportOutsideInputs(paths, out); err != nil {
		return err
	}
	return writeHistoryReport(paths, runtime, out)
}

func reportOutsideInputs(paths []string, out string) error {
	resolvedOut, err := resolveFuturePath(out)
	if err != nil {
		return err
	}
	for _, path := range paths {
		input, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		input, err = filepath.Abs(input)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(input, resolvedOut)
		if err != nil {
			return err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
			return fmt.Errorf("report output must not be inside an input bundle")
		}
	}
	return nil
}

func writeHistoryReport(paths []string, runtime, out string) error {
	var bundles []experiment.Bundle
	var receipts []analysis.HistoryEvidence
	for i, path := range paths {
		b, err := experiment.Load(path)
		if err != nil {
			return err
		}
		digest, err := experiment.DigestFile(filepath.Join(path, "checksums.json"))
		if err != nil {
			return err
		}
		bundles = append(bundles, b)
		receipts = append(receipts, analysis.HistoryEvidence{Run: b.Manifest.ID, Bundle: fmt.Sprintf("raw/%03d", i), ChecksumsSHA256: digest})
	}
	r, err := analysis.History(bundles, runtime)
	if err != nil {
		return err
	}
	r.Evidence = receipts
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	for i, path := range paths {
		dst := filepath.Join(out, receipts[i].Bundle)
		if err = os.CopyFS(dst, os.DirFS(path)); err != nil {
			return err
		}
		if _, err = experiment.Load(dst); err != nil {
			return err
		}
		digest, err := experiment.DigestFile(filepath.Join(dst, "checksums.json"))
		if err != nil {
			return err
		}
		if digest != receipts[i].ChecksumsSHA256 {
			return fmt.Errorf("history evidence changed while copying")
		}
		if err = experiment.WriteJSON(filepath.Join(out, fmt.Sprintf("trials-%03d.json", i)), bundles[i].Trials); err != nil {
			return err
		}
	}
	if err = experiment.WriteJSON(filepath.Join(out, "data.json"), r); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	t, err := template.ParseFS(historyAssets, "history.html")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	// JSON marshaling escapes HTML-sensitive data before embedding it.
	err = t.Execute(f, struct{ Data template.JS }{template.JS(data)})
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return sealFamilyReport(out, "history")
}

func resolveFuturePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, suffix := abs, ""
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			return filepath.Join(resolved, suffix), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(parent), suffix)
		parent = next
	}
}
