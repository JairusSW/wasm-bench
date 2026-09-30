package publish

import (
	"bytes"
	"compress/gzip"
	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestProfileDownloadsUnmodifiedAndNewOnly(t *testing.T) {
	var data bytes.Buffer
	z := gzip.NewWriter(&data)
	z.Write([]byte("opaque transport fixture"))
	z.Close()
	digest := corpus.Hash(data.Bytes())
	p := &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "pprof-gzip", Collector: "runtime/pprof", CollectorVersion: "test", Scope: "adapter_process_go_cpu", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: data.Bytes()}
	b := experiment.Bundle{Trials: []experiment.Trial{{CPUProfile: p}, {CPUProfile: p}, {Profile: "profiling", Status: "unsupported"}}}
	root := t.TempDir()
	if err := exportProfiles(b, root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "profiles", digest+".pprof"))
	if err != nil || !bytes.Equal(got, p.Data) {
		t.Fatal(err)
	}
	if exportProfiles(b, root) == nil {
		t.Fatal("overwrote output")
	}
	p.SHA256 = "../../escape"
	if exportProfiles(b, t.TempDir()) == nil {
		t.Fatal("unsafe digest accepted")
	}
}

func TestNativeV8ProfileDownload(t *testing.T) {
	data := []byte(`{"nodes":[],"startTime":0,"endTime":1}`)
	digest := corpus.Hash(data)
	p := &protocol.CPUProfile{Version: 1, ModuleSHA256: digest, Format: "v8-cpuprofile-json", Collector: "node:inspector/Profiler", CollectorVersion: "test", Scope: "adapter_v8_isolate_sampled_stacks", Window: "run_request_including_setup_warmup_verification", Quality: "sampled", Status: "collected", SHA256: digest, Data: data}
	root := t.TempDir()
	if err := exportProfiles(experiment.Bundle{Trials: []experiment.Trial{{CPUProfile: p}}}, root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "profiles", digest+".cpuprofile"))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("native bytes changed", err)
	}
	if _, err = os.Stat(filepath.Join(root, "profiles", digest+".pprof")); !os.IsNotExist(err) {
		t.Fatal("V8 mislabeled as pprof", err)
	}
}
