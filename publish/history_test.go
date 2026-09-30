package publish

import (
	"bytes"
	"encoding/json"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryRejectsNestedOutputBeforeWriting(t *testing.T) {
	input := t.TempDir()
	out := filepath.Join(input, "new", "report")
	if err := HistoryReport([]string{input}, "runtime", out); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(out)); !os.IsNotExist(err) {
		t.Fatal("input modified", err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(input, alias); err != nil {
		t.Skip(err)
	}
	if err := HistoryReport([]string{input}, "runtime", filepath.Join(alias, "report")); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatal(err)
	}
}

func TestHistoryTemplateEscapesEmbeddedData(t *testing.T) {
	tpl, err := template.ParseFS(historyAssets, "history.html")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"runtime": "</script><script>alert(1)</script>"})
	var out bytes.Buffer
	if err := tpl.Execute(&out, struct{ Data template.JS }{template.JS(data)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "</script><script>alert") || !strings.Contains(out.String(), `\u003c/script\u003e`) {
		t.Fatal("unsafe embedded data")
	}
}
