package sourcebuild

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/wasmbench/wasmbench/protocol"
)

// SameTask establishes the fixed side of a source-toolchain comparison.
// Source locators may move; staged file names and content hashes may not.
// Tool binaries, flags and build environment are deliberately variable.
func SameTask(a, b Result) error {
	x, y := a.Lock.Recipe, b.Lock.Recipe
	if x.SourceRevision != y.SourceRevision || x.License != y.License {
		return fmt.Errorf("source revision or license differs")
	}
	inputs := func(r Recipe) map[string]string {
		out := map[string]string{}
		for name, file := range r.Inputs {
			out[name] = file.SHA256
		}
		return out
	}
	if !reflect.DeepEqual(inputs(x), inputs(y)) {
		return fmt.Errorf("source input names or hashes differ")
	}
	xb, _ := json.Marshal(x.Workload)
	yb, _ := json.Marshal(y.Workload)
	if string(xb) != string(yb) {
		return fmt.Errorf("source workload contract differs")
	}
	if a.Lock.Analyzer == nil || b.Lock.Analyzer == nil {
		return fmt.Errorf("source analyzer policy missing")
	}
	ax, ay := *a.Lock.Analyzer, *b.Lock.Analyzer
	ax.Executable, ay.Executable = "", ""
	if ax != ay {
		return fmt.Errorf("source validation analyzer or feature policy differs")
	}
	return nil
}

// MatchesWorkload binds runtime observations to this exact source-build output
// and contract. Only the artifact locator may differ in a portable run bundle.
func (r Result) MatchesWorkload(w protocol.Workload) bool {
	want := emittedWorkload(r, "")
	want.Artifact = w.Artifact
	a, _ := json.Marshal(want)
	b, _ := json.Marshal(w)
	return string(a) == string(b)
}
