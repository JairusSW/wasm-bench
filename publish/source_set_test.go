package publish

import (
	"bytes"
	"encoding/json"
	"html/template"
	"strings"
	"testing"
)

func TestSplitSourceBuildPaths(t *testing.T) {
	for _, input := range []string{"", "one,,two", "one,  ,two"} {
		if _, err := SplitSourceBuildPaths(input); err == nil {
			t.Fatalf("accepted malformed list %q", input)
		}
	}
	got, err := SplitSourceBuildPaths(" one ,two ")
	if err != nil || len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("split paths: %#v, %v", got, err)
	}
}

func TestSourceSetTemplateEscapesEmbeddedData(t *testing.T) {
	tpl, err := template.ParseFS(sourceSetAssets, "source_set.html")
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
