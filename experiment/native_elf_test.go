package experiment

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/agent"
	"github.com/wasmbench/wasmbench/corpus"
)

func TestELFLoaderOutput(t *testing.T) {
	loader := "/lib/ld-linux-aarch64.so.1"
	valid := " (0x400000)\nlinux-vdso.so.1 (0xabcd)\nliba.so => /path with spaces/liba.so (0xabcd)\n" + loader + " (0xaabb)\n"
	got, err := parseELFList(valid, loader)
	if err != nil || !reflect.DeepEqual(got, map[string]string{"liba.so": "/path with spaces/liba.so"}) {
		t.Fatal(got, err)
	}
	if _, err := parseELFList(strings.ReplaceAll(valid, "/path with spaces/liba.so", "/app/bin/../lib/liba.so"), loader); err != nil {
		t.Fatal("rejected normal ORIGIN path", err)
	}
	for _, bad := range []string{"", "liba.so => not found\n", "warning: anything\n", strings.ReplaceAll(valid, "0xaabb", "0xG"), strings.ReplaceAll(valid, "/path with spaces/liba.so", "relative/liba.so"), valid + "liba.so => /different/liba.so (0xabcd)\n"} {
		if _, err := parseELFList(bad, loader); err == nil {
			t.Fatal("accepted ambiguous/incomplete loader output", bad)
		}
	}
}

func TestELFLoaderOverridePolicy(t *testing.T) {
	for _, name := range []string{"LD_PRELOAD", "LD_AUDIT", "LD_LIBRARY_PATH", "LD_DEBUG_OUTPUT", "GLIBC_TUNABLES"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "not-qualified")
			if err := elfEnvironment(); err == nil {
				t.Fatal("accepted loader override")
			}
		})
	}
}

func TestELFContractRejectsMissingAndUnboundEvidence(t *testing.T) {
	if err := validateELFContract(Runtime{NativeDependencyPolicy: elfDependencyPolicy}); err == nil {
		t.Fatal("accepted missing evidence")
	}
	digest := strings.Repeat("a", 64)
	makeRuntime := func() Runtime {
		return Runtime{NativeDependencyPolicy: elfDependencyPolicy, Files: map[string]string{"/lib/libc.so": digest}, HostFiles: map[string]string{"/lib/loader.so": digest}, ELF: &ELFDependencies{Version: elfDependencyVersion, Interpreter: NativeLibrary{Path: "/lib/loader.so", SHA256: digest}, Libraries: map[string]NativeLibrary{"libc.so": {Path: "/lib/libc.so", SHA256: digest}}, ControlFiles: map[string]string{"/etc/ld.so.cache": "absent", "/etc/ld.so.preload": "absent"}}}
	}
	if err := validateELFContract(makeRuntime()); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"library", "loader", "control", "version", "policy"} {
		r := makeRuntime()
		switch mode {
		case "library":
			r.Files = map[string]string{}
		case "loader":
			r.HostFiles = map[string]string{}
		case "control":
			r.ELF.ControlFiles["/etc/ld.so.cache"] = digest
		case "version":
			r.ELF.Version = "unknown"
		case "policy":
			r.NativeDependencyPolicy = "unknown"
		}
		if err := validateELFContract(r); err == nil {
			t.Fatal("accepted incomplete evidence", mode)
		}
	}
}

func TestELFLiveStartupDependencies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native Linux loader required")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is the dynamic ELF fixture")
	}
	digest, err := DigestFile(node)
	if err != nil {
		t.Fatal(err)
	}
	r := Runtime{ID: "node", Command: []string{node}, Files: map[string]string{node: digest}}
	if err = pinNativeDependencies(&r); err != nil {
		t.Fatal(err)
	}
	if r.ELF == nil || r.ELF.Interpreter.Path == "" || len(r.ELF.Libraries) == 0 {
		t.Fatal("did not capture dynamic dependencies", r.ELF)
	}
	if err = VerifyInputs(Lock{Runtimes: []Runtime{r}}, "."); err != nil {
		t.Fatal(err)
	}
	if err = qualifyELFRuntime(context.Background(), &r); err != nil {
		t.Fatal(err)
	}
	// A well-formed but wrong locked library hash must fail actual resolution.
	for name, lib := range r.ELF.Libraries {
		lib.SHA256 = strings.Repeat("0", 64)
		r.ELF.Libraries[name] = lib
		break
	}
	if err = qualifyELFRuntime(context.Background(), &r); err == nil {
		t.Fatal("accepted changed startup library")
	}
}

func TestELFStaticAndMalformed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(path, []byte("\x7fELFbroken"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, recognized, err := elfInterpreter(path); !recognized || err == nil {
		t.Fatal("silently ignored invalid ELF", err)
	}
	if runtime.GOOS != "linux" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := probeELF(context.Background(), executable)
	if err != nil || probe == nil {
		t.Fatal(probe, err)
	}
}

func TestELFOriginArchiveRelocation(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("native Linux fixture")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("C compiler required for origin-relative shared library fixture")
	}
	tmp := t.TempDir()
	app := filepath.Join(tmp, "app")
	for _, dir := range []string{"bin", "lib"} {
		if err = os.MkdirAll(filepath.Join(app, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(tmp, "answer.c")
	mainSource := filepath.Join(tmp, "main.c")
	if err = os.WriteFile(source, []byte("int answer(void) { return 42; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(mainSource, []byte("extern int answer(void); int main(void) { return answer() != 42; }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	library := filepath.Join(app, "lib", "libanswer.so")
	executable := filepath.Join(app, "bin", "fixture")
	for _, args := range [][]string{{"-shared", "-fPIC", source, "-o", library}, {mainSource, "-L" + filepath.Join(app, "lib"), "-lanswer", "-Wl,-rpath,$ORIGIN/../lib", "-o", executable}} {
		if out, err := exec.Command(cc, args...).CombinedOutput(); err != nil {
			t.Fatalf("compile fixture: %v %s", err, out)
		}
	}
	digest, _ := DigestFile(executable)
	r := Runtime{ID: "origin", Command: []string{executable}, Files: map[string]string{executable: digest}}
	if err = pinNativeDependencies(&r); err != nil {
		t.Fatal(err)
	}
	if r.Files[library] == "" {
		t.Fatal("origin library missing", r)
	}
	bundle := filepath.Join(tmp, "bundle")
	if err = os.Mkdir(bundle, 0755); err != nil {
		t.Fatal(err)
	}
	workloads, err := corpus.Generate(bundle, "core")
	if err != nil {
		t.Fatal(err)
	}
	for i := range workloads {
		workloads[i].Artifact, err = filepath.Rel(bundle, workloads[i].Artifact)
		if err != nil {
			t.Fatal(err)
		}
	}
	lock, err := NewLock(Options{Profile: "timing", Scenarios: []string{"first-call"}, Samples: 1, Operations: 1, Launches: 1, Timeout: time.Second}, []Runtime{r}, workloads)
	if err != nil {
		t.Fatal(err)
	}
	lock.ArchiveTools = true
	runner, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = archiveTools(lock, runner, bundle); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(lock)
	if err = WriteJSON(filepath.Join(bundle, "manifest.json"), Manifest{Lock: lock, LockSHA256: corpus.Hash(encoded), Host: agent.IdentifyHost()}); err != nil {
		t.Fatal(err)
	}
	if err = Seal(bundle); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(app, filepath.Join(tmp, "original-no-longer-at-locked-path")); err != nil {
		t.Fatal(err)
	}
	restoration, err := RestoreTools(bundle, filepath.Join(tmp, "restored"))
	if err != nil {
		t.Fatal(err)
	}
	var restored Lock
	if err = ReadJSON(restoration.Lock, &restored); err != nil {
		t.Fatal(err)
	}
	if err = qualifyELFRuntime(context.Background(), &restored.Runtimes[0]); err != nil {
		t.Fatal(err)
	}
	actual := restored.Runtimes[0].ELF.Libraries["libanswer.so"]
	if !strings.HasPrefix(actual.Path, filepath.Dir(restoration.Lock)+string(filepath.Separator)) {
		t.Fatal("still loaded original origin library", actual)
	}
	if out, err := exec.Command(restored.Runtimes[0].Command[0]).CombinedOutput(); err != nil {
		t.Fatalf("restored invocation: %v %s", err, out)
	}
}
