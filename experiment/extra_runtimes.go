package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/wasmbench/wasmbench/protocol"
)

// Shell paths are resolved before locking; changing PATH later cannot replace
// the engine in a locked experiment. No installer or jsvu is required.
func extraRuntimeCommand(root, id string) ([]string, bool, error) {
	if id == "chicory" {
		java := os.Getenv("WASMBENCH_JAVA")
		if java == "" {
			java = "java"
		}
		path, err := exec.LookPath(java)
		if err != nil {
			return nil, true, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, true, err
		}
		return []string{path, "-jar", filepath.Join(root, "bin", "adapter-chicory.jar")}, true, nil
	}
	if id == "jsc" && os.Getenv("WASMBENCH_JSC_HOST") != "" {
		path, err := exec.LookPath(os.Getenv("WASMBENCH_JSC_HOST"))
		if err != nil {
			return nil, true, fmt.Errorf("jsc embedding host: %w", err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, true, err
		}
		digest, err := DigestFile(path)
		if err != nil {
			return nil, true, err
		}
		args := []string{path, filepath.Join(root, "adapters", "js-shell", "adapter.js"), "--", "runtime=jsc", "binary-sha256=" + digest, "tier-mode=omg-eager"}
		if version := os.Getenv("WASMBENCH_JSC_VERSION"); version != "" {
			args = append(args, "runtime-version="+version)
		}
		return args, true, nil
	}
	tools := map[string]struct{ env, executable string }{
		"spidermonkey": {"WASMBENCH_SPIDERMONKEY", "spidermonkey"},
		"jsc":          {"WASMBENCH_JSC", "jsc"},
		"v8-shell":     {"WASMBENCH_V8_SHELL", "d8"},
		"deno":         {"WASMBENCH_DENO", "deno"},
	}
	if tool, ok := tools[id]; ok {
		path := os.Getenv(tool.env)
		if path == "" {
			if id == "jsc" && runtime.GOOS == "darwin" {
				path = "/System/Library/Frameworks/JavaScriptCore.framework/Versions/A/Helpers/jsc"
			} else {
				path = tool.executable
			}
		}
		path, err := exec.LookPath(path)
		if err != nil {
			return nil, true, fmt.Errorf("%s: install %s or set %s to its executable: %w", id, tool.executable, tool.env, err)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, true, err
		}
		digest, err := DigestFile(path)
		if err != nil {
			return nil, true, err
		}
		script := filepath.Join(root, "adapters", "js-shell", "adapter.js")
		args := []string{path}
		switch id {
		case "jsc":
			args = append(args, "--validateOptions=true", "--thresholdForBBQOptimizeAfterWarmUp=1", "--thresholdForBBQOptimizeSoon=1", "--thresholdForOMGOptimizeAfterWarmUp=1", "--thresholdForOMGOptimizeSoon=1", "--useConcurrentJIT=false", "--numberOfWasmCompilerThreads=0", "--dumpOMGDisassembly=true", script, "--")
		case "deno":
			args = append(args, "run", "--v8-flags=--allow-natives-syntax,--no-liftoff,--no-wasm-tier-up,--no-wasm-lazy-compilation", "--no-prompt", "--allow-read", script)
		case "spidermonkey":
			args = append(args, "--wasm-compiler=ion", "-f", script, "--")
		default:
			args = append(args, script, "--")
		}
		args = append(args, "runtime="+id, "binary-sha256="+digest)
		switch id {
		case "spidermonkey":
			args = append(args, "tier-mode=ion-only")
		case "jsc":
			args = append(args, "tier-mode=omg-eager")
		case "deno":
			args = append(args, "tier-mode=optimizing-only")
		}
		if id == "jsc" && os.Getenv("WASMBENCH_JSC_VERSION") != "" {
			args = append(args, "runtime-version="+os.Getenv("WASMBENCH_JSC_VERSION"))
		}
		return args, true, nil
	}
	switch id {
	case "wasmi", "wavm", "wamr", "wasm3", "wasmedge", "wasmer-llvm", "wasmer-singlepass":
		return []string{NativeExecutable(filepath.Join(root, "bin", "adapter-"+id))}, true, nil
	}
	return nil, false, nil
}

func validateExtraProbe(data []byte) error {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		return fmt.Errorf("expected two JSON protocol responses, got %d", len(lines))
	}
	for i, line := range lines {
		var r protocol.Response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return fmt.Errorf("invalid shell control response: %w", err)
		}
		if r.Version != protocol.Version || r.ID != i+1 || r.Status != "ok" || (i == 0 && (r.Description == nil || r.Description.Runtime == "")) {
			return fmt.Errorf("shell protocol probe failed: %s", line)
		}
	}
	return nil
}

func verifyJSCOMG(ctx context.Context) error {
	executable := os.Getenv("WASMBENCH_JSC")
	if executable == "" {
		if runtime.GOOS == "darwin" {
			executable = "/System/Library/Frameworks/JavaScriptCore.framework/Versions/A/Helpers/jsc"
		} else {
			executable = "jsc"
		}
	}
	path, err := exec.LookPath(executable)
	if err != nil {
		return fmt.Errorf("JSC OMG verification shell: %w", err)
	}
	program := `const m=new WebAssembly.Module(Uint8Array.from([0,97,115,109,1,0,0,0,1,5,1,96,0,1,127,3,2,1,0,7,8,1,4,116,101,115,116,0,0,10,6,1,4,0,65,42,11]));const f=new WebAssembly.Instance(m).exports.test;for(let i=0;i<100000;i++)f();`
	args := []string{"--validateOptions=true", "--thresholdForBBQOptimizeAfterWarmUp=1", "--thresholdForBBQOptimizeSoon=1", "--thresholdForOMGOptimizeAfterWarmUp=1", "--thresholdForOMGOptimizeSoon=1", "--useConcurrentJIT=false", "--numberOfWasmCompilerThreads=0", "--dumpOMGDisassembly=true", "-e", program}
	command := exec.CommandContext(ctx, path, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("JSC OMG tier probe: %w: %s", err, output)
	}
	if !strings.Contains(string(output), "Generated OMG") {
		return fmt.Errorf("JSC highest-tier probe did not emit OMG compilation evidence: %s", output)
	}
	return nil
}

// BuildExtraRuntime uses an installed shell or explicitly selected native SDK.
// It never silently falls back to another runtime or downloads SDKs.
func BuildExtraRuntime(ctx context.Context, root, id string) (bool, error) {
	if !slices.Contains([]string{"wasmi", "wavm", "wamr", "wasm3", "wasmedge", "wasmer-llvm", "wasmer-singlepass", "spidermonkey", "jsc", "v8-shell", "deno", "chicory"}, id) {
		return false, nil
	}
	if id == "jsc" && (runtime.GOOS == "linux" || os.Getenv("WASMBENCH_JSC_HOST") != "") {
		compiler := os.Getenv("CXX")
		if compiler == "" {
			compiler = "c++"
		}
		compiler, err := exec.LookPath(compiler)
		if err != nil {
			return true, fmt.Errorf("jsc host C++ compiler: %w", err)
		}
		output := filepath.Join(root, "bin", "adapter-jsc")
		if err = os.MkdirAll(filepath.Dir(output), 0755); err != nil {
			return true, err
		}
		args := []string{"-std=c++17", "-O2", filepath.Join(root, "adapters", "js-shell", "jsc-host.cpp"), "-o", output}
		if runtime.GOOS == "darwin" {
			args = append(args, "-framework", "JavaScriptCore")
		} else if runtime.GOOS == "linux" {
			sdk := os.Getenv("WASMBENCH_JSC_SDK")
			if sdk == "" {
				return true, fmt.Errorf("set WASMBENCH_JSC_SDK to the extracted JavaScriptCoreGTK development package")
			}
			include := filepath.Join(sdk, "usr", "include", "webkitgtk-4.1")
			if _, err = os.Stat(filepath.Join(include, "JavaScriptCore", "JavaScript.h")); err != nil {
				return true, fmt.Errorf("JavaScriptCoreGTK headers missing from %s: %w", include, err)
			}
			library := os.Getenv("WASMBENCH_JSC_LIBRARY")
			if library == "" {
				library = "/usr/lib/x86_64-linux-gnu/libjavascriptcoregtk-4.1.so.0"
			}
			if _, err = os.Stat(library); err != nil {
				return true, fmt.Errorf("JavaScriptCoreGTK runtime library missing at %s: %w", library, err)
			}
			args = append(args, "-I"+include, library)
		} else {
			return true, fmt.Errorf("jsc embedding host is not qualified for %s", runtime.GOOS)
		}
		cmd := exec.CommandContext(ctx, compiler, args...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = root, os.Stdout, os.Stderr
		if err = cmd.Run(); err != nil {
			return true, fmt.Errorf("build jsc embedding host: %w", err)
		}
		if err = os.Setenv("WASMBENCH_JSC_HOST", output); err != nil {
			return true, err
		}
	}
	switch id {
	case "wasmi", "wavm", "wamr", "wasm3", "wasmedge", "wasmer-llvm", "wasmer-singlepass":
		cmd := exec.CommandContext(ctx, "cargo", "build", "--release", "--locked", "--no-default-features", "--features", strings.ReplaceAll(id, "-", "_"), "--target-dir", filepath.Join(root, "adapters", "native", "target", id))
		cmd.Dir = filepath.Join(root, "adapters", "native")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return true, err
		}
		source := NativeExecutable(filepath.Join(root, "adapters", "native", "target", id, "release", "adapter-native"))
		data, err := os.ReadFile(source)
		if err != nil {
			return true, err
		}
		if err = os.MkdirAll(filepath.Join(root, "bin"), 0755); err != nil {
			return true, err
		}
		return true, os.WriteFile(NativeExecutable(filepath.Join(root, "bin", "adapter-"+id)), data, 0755)
	default:
		command, _, err := extraRuntimeCommand(root, id)
		if err != nil {
			return true, err
		}
		// Check actual protocol support, not just executable presence or JS syntax.
		probeContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(probeContext, command[0], command[1:]...)
		cmd.Stdin = strings.NewReader("{\"version\":1,\"id\":1,\"method\":\"describe\"}\n{\"version\":1,\"id\":2,\"method\":\"close\"}\n")
		var stderr strings.Builder
		cmd.Stderr = &stderr
		data, err := cmd.Output()
		if err != nil {
			return true, fmt.Errorf("%s protocol probe: %w", id, err)
		}
		if err := validateExtraProbe(data); err != nil {
			return true, err
		}
		if id == "jsc" {
			if os.Getenv("WASMBENCH_JSC_HOST") != "" {
				if err := verifyJSCOMG(probeContext); err != nil {
					return true, err
				}
			} else if !strings.Contains(stderr.String(), "OMG") {
				return true, fmt.Errorf("jsc highest-tier preflight did not emit OMG compilation evidence: %s", stderr.String())
			}
		}
		return true, nil
	}
}
