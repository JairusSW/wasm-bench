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

func TestSourceReportRejectsNestedOutput(t *testing.T) {
	input := t.TempDir()
	out := filepath.Join(input, "nested", "report")
	if err := SourceReport(input, out); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(out)); !os.IsNotExist(err) {
		t.Fatal("input modified", err)
	}
}

func TestSourceReportTemplateEscapesData(t *testing.T) {
	tpl, err := template.ParseFS(sourceAssets, "source.html")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]string{"id": "</script><script>alert(1)</script>"})
	var out bytes.Buffer
	if err := tpl.Execute(&out, struct{ Data template.JS }{template.JS(data)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "</script><script>alert") || !strings.Contains(out.String(), `\u003c/script\u003e`) {
		t.Fatal("unsafe embedded data")
	}
}
