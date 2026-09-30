package experiment

import (
	"bytes"
	"compress/gzip"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCPUProfileTransportEvidence(t *testing.T) {
	for _, mode := range []string{"valid", "failed", "corrupt", "wrong-module", "timing", "admission", "unknown-workload"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "trials"), 0755); err != nil {
				t.Fatal(err)
			}
			var data bytes.Buffer
			z := gzip.NewWriter(&data)
			z.Write([]byte("opaque transport fixture"))
			z.Close()
			digest := corpus.Hash(data.Bytes())
			p := &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "pprof-gzip", Collector: "runtime/pprof", CollectorVersion: "test", Scope: "adapter_process_go_cpu", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: data.Bytes()}
			m := Manifest{Lock: Lock{Workloads: []protocol.Workload{{ID: "w", SHA256: digest}}}}
			trial := Trial{ID: "t", Workload: "w", Profile: "profiling", Status: "ok", CPUProfile: p}
			switch mode {
			case "failed":
				trial.Status = "incorrect_result"
			case "corrupt":
				p.Data = []byte("corrupt")
			case "wrong-module":
				p.ModuleSHA256 = corpus.Hash([]byte("other"))
			case "timing":
				trial.Profile = "timing"
			case "admission":
				trial.Block = -1
			case "unknown-workload":
				trial.Workload = "missing"
			}
			if err := WriteJSON(filepath.Join(root, "manifest.json"), m); err != nil {
				t.Fatal(err)
			}
			if err := WriteJSON(filepath.Join(root, "trials/t.json"), trial); err != nil {
				t.Fatal(err)
			}
			if err := Seal(root); err != nil {
				t.Fatal(err)
			}
			b, err := Load(root)
			if mode == "valid" || mode == "failed" {
				if err != nil || len(b.Trials) != 1 || b.Trials[0].CPUProfile == nil {
					t.Fatal(b, err)
				}
			} else if err == nil {
				t.Fatal("invalid resealed profile accepted")
			}
		})
	}
}
