package publish

import (
	"bytes"
	"encoding/json"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareReportRejectsNestedOutput(t *testing.T) {
	input := t.TempDir()
	out := filepath.Join(input, "report")
	if err := CompareReport(input, input, "baseline", "candidate", out); err == nil || !strings.Contains(err.Error(), "inside") {
		t.Fatal(err)
	}
}

func TestCompareReportTemplateEscapesEmbeddedData(t *testing.T) {
	tpl, err := template.ParseFS(compareReportAssets, "compare_report.html")
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]string{"value": "</script><script>alert(1)</script>"})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err = tpl.Execute(&out, struct{ Data template.JS }{template.JS(data)}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "</script><script>alert") || !strings.Contains(out.String(), `\u003c/script\u003e`) {
		t.Fatal("unsafe embedded report JSON")
	}
}
