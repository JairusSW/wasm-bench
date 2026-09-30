package experiment

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedPilotBudgetCannotChange(t *testing.T) {
	p := map[string]any{"version": PilotVersion, "status": "ready", "chosen_launches": 12, "min_launches": 6, "max_launches": 100, "target_relative_ci_half_width": .05, "source_lock_sha256": strings.Repeat("a", 64), "source_checksums_sha256": strings.Repeat("b", 64)}
	data, _ := json.Marshal(p)
	l := Lock{PilotPlan: data, Options: Options{Profile: "timing", Launches: 12}}
	if err := validatePilotPlan(l); err != nil {
		t.Fatal(err)
	}
	l.Options.Launches = 13
	if validatePilotPlan(l) == nil {
		t.Fatal("budget override accepted")
	}
	root := t.TempDir()
	if err := WriteJSON(filepath.Join(root, "manifest.json"), Manifest{Lock: l}); err != nil {
		t.Fatal(err)
	}
	if err := Seal(root); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("resealed confirmation budget override accepted")
	}
	l.Options.Launches = 12
	l.Options.Check = true
	if validatePilotPlan(l) == nil {
		t.Fatal("correctness plan accepted")
	}
	l.Options.Check = false
	p["status"] = "unresolved"
	l.PilotPlan, _ = json.Marshal(p)
	if validatePilotPlan(l) == nil {
		t.Fatal("unresolved plan accepted")
	}
}
