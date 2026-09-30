package experiment

import (
	"encoding/json"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"testing"
)

func TestCodeLifetimeSealedEvidence(t *testing.T) {
	root := os.Getenv("WASMBENCH_CODE_LIFETIME_BUNDLE")
	if root == "" {
		t.Skip("set WASMBENCH_CODE_LIFETIME_BUNDLE to a sealed Linux core code lifetime run")
	}
	b, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, trial := range b.Trials {
		if trial.Block < 0 {
			continue
		}
		if trial.Status != "ok" || trial.CodeLifetime == nil || len(trial.Samples) != 0 {
			t.Fatalf("incomplete diagnostic trial: %s %s", trial.ID, trial.Status)
		}
		count++
	}
	if count != 12 {
		t.Fatalf("expected 2 workloads x 2 backends x 3 launches; got %d", count)
	}
	clone := func() Bundle {
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var copied Bundle
		if err = json.Unmarshal(raw, &copied); err != nil {
			t.Fatal(err)
		}
		return copied
	}
	for _, mode := range []string{"missing", "wrong_result", "host", "host_page_size", "missing_host_page_size", "backend", "runtime", "config", "input", "image", "early_retirement", "foreign", "timing_samples", "coverage", "admission"} {
		t.Run(mode, func(t *testing.T) {
			bad := clone()
			i := 0
			for bad.Trials[i].CodeLifetime == nil {
				i++
			}
			trial := &bad.Trials[i]
			switch mode {
			case "missing":
				trial.CodeLifetime = nil
			case "wrong_result":
				trial.CodeLifetime.BeforeDrop[0]++
				trial.CodeLifetime.AfterDrop[0]++
			case "host":
				bad.Manifest.Host.OS = "darwin"
			case "host_page_size":
				bad.Manifest.Host.PageSize *= 2
			case "missing_host_page_size":
				bad.Manifest.Host.PageSize = 0
			case "backend":
				if trial.CodeLifetime.Backend == "cranelift" {
					trial.CodeLifetime.Backend = "winch"
				} else {
					trial.CodeLifetime.Backend = "cranelift"
				}
			case "runtime":
				bad.Manifest.Lock.Runtimes[0].Description.Version = "other"
			case "config":
				for i := range bad.Manifest.Lock.Runtimes {
					bad.Manifest.Lock.Runtimes[i].Description.Configuration["code_lifetime"] = "inferred"
				}
			case "input":
				bad.Manifest.Lock.Workloads[0].SHA256 = "other"
			case "image":
				trial.CodeImage = nil
			case "early_retirement":
				trial.CodeLifetime.Checkpoints[2].EventCount = 2
			case "foreign":
				trial.Scenario = "compile"
			case "timing_samples":
				trial.Samples = []protocol.Sample{{Verified: true}}
			case "coverage":
				trial.CodeImage.Functions = nil
			case "admission":
				bad.Admission = nil
			}
			if ValidateCodeLifetimeEvidence(root, bad) == nil {
				t.Fatal("forged evidence accepted")
			}
		})
	}
}
