package corpus

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestScalingBounds(t *testing.T) {
	for _, n := range []int{-1, 0, 65537} {
		if _, err := ScalingModule("locals", n); err == nil {
			t.Fatalf("accepted size %d", n)
		}
	}
	if _, err := ScalingModule("unknown", 1); err == nil {
		t.Fatal("accepted unknown dimension")
	}
}

// Independent wasmparser evidence guards against fixtures that grow in bytes
// while failing to grow the dimension advertised in their manifest.
func TestScalingIndependentAnalyzer(t *testing.T) {
	analyzer, err := filepath.Abs("../adapters/wasmtime/target/release/wasm-analyze")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(analyzer); os.IsNotExist(err) {
		t.Skip("build the pinned wasmparser analyzer to run structural admission")
	}
	workloads, err := Generate(t.TempDir(), "scaling")
	if err != nil {
		t.Fatal(err)
	}
	if len(workloads) != 40 {
		t.Fatal("incomplete scaling matrix")
	}
	for _, w := range workloads {
		t.Run(w.ID, func(t *testing.T) {
			data, err := exec.Command(analyzer, w.Artifact).CombinedOutput()
			if err != nil {
				t.Fatalf("analyzer: %v\n%s", err, data)
			}
			var s struct {
				Validated bool  `json:"validated"`
				Functions int   `json:"defined_functions"`
				Types     int   `json:"type_groups"`
				Imports   int   `json:"import_groups"`
				Data      int   `json:"data_segments"`
				Depth     int   `json:"max_control_depth"`
				Tables    []int `json:"branch_table_entries"`
				Bodies    []struct {
					Locals    int `json:"locals"`
					Operators int `json:"operators"`
				} `json:"functions"`
			}
			if err = json.Unmarshal(data, &s); err != nil {
				t.Fatal(err)
			}
			if !s.Validated {
				t.Fatal("not validated")
			}
			got, want := 0, w.Size
			switch w.Dimension {
			case "functions":
				got = s.Functions
			case "body":
				got = s.Bodies[0].Operators
				want = 2*w.Size + 2
			case "locals":
				got = s.Bodies[0].Locals
			case "nesting":
				got = s.Depth
			case "branch-table":
				if len(s.Tables) != 1 {
					t.Fatal(s.Tables)
				}
				got = s.Tables[0]
			case "types":
				got = s.Types
				want = w.Size + 1
			case "data-segments":
				got = s.Data
			case "imports":
				got = s.Imports
			}
			if got != want {
				t.Fatalf("dimension %s got %d want %d", w.Dimension, got, want)
			}
		})
	}
}
