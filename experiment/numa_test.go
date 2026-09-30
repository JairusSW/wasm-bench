package experiment

import (
	"github.com/wasmbench/wasmbench/agent"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadNUMAReadbackCannotBeOmittedOrForged(t *testing.T) {
	for _, mode := range []string{"valid", "missing", "forged", "wrong-policy"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			p := agent.ResourcePolicy{CgroupParent: "/cg", Mems: "0"}
			values := map[string]string{"cpuset.mems": "0", "cpuset.mems.effective": "0"}
			a := agent.CheckResourceReadback(p, values, "before_spawn")
			z := agent.CheckResourceReadback(p, values, "response_end_before_cleanup")
			iso := &agent.Isolation{Mode: "cgroup_v2_at_spawn", Path: "/cg/leaf", Effective: values, Verification: &a, FinalVerification: &z}
			switch mode {
			case "missing":
				iso.FinalVerification = nil
			case "forged":
				z.Effective["cpuset.mems.effective"] = "1"
			case "wrong-policy":
				p.Mems = "1"
			}
			if err := WriteJSON(filepath.Join(root, "manifest.json"), Manifest{Lock: Lock{Options: Options{Resources: p}}}); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(root, "trials", "t.json"), Trial{ID: "t", Status: "ok", Isolation: iso}); err != nil {
				t.Fatal(err)
			}
			if err := Seal(root); err != nil {
				t.Fatal(err)
			}
			_, err := Load(root)
			if (err == nil) != (mode == "valid") {
				t.Fatal(mode, err)
			}
		})
	}
}
