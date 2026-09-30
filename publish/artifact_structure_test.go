package publish

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wasmbench/wasmbench/experiment"
)

func TestArtifactStructuresProjectSealedAnalyzerEvidence(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "validation"), 0700); err != nil {
		t.Fatal(err)
	}
	coreDigest, componentDigest := strings.Repeat("a", 64), strings.Repeat("b", 64)
	for digest, body := range map[string]string{
		coreDigest:      `{"encoding":"core-module","bytes":1234,"section_payload_bytes":{"10":450},"data_bytes":40,"custom_data_bytes":80,"debug_data_bytes":70,"defined_functions":4,"total_functions":6,"import_count":2,"exports":3,"max_control_depth":7,"functions":[{"body_bytes":100},{"body_bytes":20},{"body_bytes":300},{"body_bytes":30}],"opcode_histogram":{"B":4,"A":4,"C":3,"D":2,"E":1,"F":1,"G":1}}`,
		componentDigest: `{"encoding":"component","bytes":99,"nodes":[{"encoding":"component"},{"encoding":"core-module"},{"encoding":"component"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(root, "validation", digest+".json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	admissions := []experiment.ArtifactAdmission{
		{SHA256: coreDigest, Status: "validated", ReportPath: "validation/" + coreDigest + ".json"},
		{SHA256: componentDigest, Status: "validated", ReportPath: "validation/" + componentDigest + ".json"},
		{SHA256: strings.Repeat("c", 64), Status: "not_recorded"},
	}
	got, err := artifactStructures(root, admissions)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatal(got)
	}
	core := got[0]
	if core.Bytes != 1234 || core.CodeSectionBytes != 450 || core.DefinedFunctions != 4 || core.TotalFunctions != 6 || core.Imports != 2 || core.Exports != 3 || core.MaxControlDepth != 7 {
		t.Fatal(core)
	}
	if core.BodyBytesP50 != 30 || core.BodyBytesP95 != 300 || core.BodyBytesMax != 300 {
		t.Fatal("wrong function-body distribution", core)
	}
	encoded, err := json.Marshal(core)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"nested_core_modules":0`) {
		t.Fatal("zero-valued fields must remain explicit for the report renderer", string(encoded))
	}
	if !reflect.DeepEqual(core.TopOpcodes, []OpcodeCount{{"A", 4}, {"B", 4}, {"C", 3}, {"D", 2}, {"E", 1}, {"F", 1}}) {
		t.Fatal("top operators must be deterministic and bounded", core.TopOpcodes)
	}
	if got[1].ComponentNodes != 3 || got[1].NestedCoreModules != 1 || got[1].DefinedFunctions != 0 || got[1].Encoding != "component" {
		t.Fatal("component must not inherit root core-module statistics", got[1])
	}
	if got[2].Status != "not_recorded" || got[2].ReportPath != "" {
		t.Fatal("missing analysis must stay missing", got[2])
	}
	admissions[0].ReportPath = "../manifest.json"
	if _, err := artifactStructures(root, admissions); err == nil {
		t.Fatal("accepted arbitrary analyzer evidence path")
	}
}
