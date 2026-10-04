package experiment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtraRuntimeCommands(t *testing.T) {
	root := t.TempDir()
	shell := filepath.Join(root, "engine")
	if err := os.WriteFile(shell, []byte("test engine"), 0755); err != nil {
		t.Fatal(err)
	}
	for id, key := range map[string]string{"spidermonkey": "WASMBENCH_SPIDERMONKEY", "jsc": "WASMBENCH_JSC", "v8-shell": "WASMBENCH_V8_SHELL", "deno": "WASMBENCH_DENO"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv(key, shell)
			if id == "jsc" {
				t.Setenv("WASMBENCH_JSC_VERSION", "WebKitGTK/2.52.6")
			}
			args, ok, err := extraRuntimeCommand(root, id)
			if err != nil || !ok {
				t.Fatal(args, ok, err)
			}
			if args[0] != shell || !strings.Contains(strings.Join(args, " "), "runtime="+id) || !strings.Contains(strings.Join(args, " "), "binary-sha256=") {
				t.Fatal(args)
			}
			if id == "deno" && (!strings.Contains(strings.Join(args, " "), "--v8-flags=--allow-natives-syntax,--no-liftoff,--no-wasm-tier-up,--no-wasm-lazy-compilation") || !strings.Contains(strings.Join(args, " "), "--no-prompt --allow-read")) {
				t.Fatal(args)
			}
			if id == "spidermonkey" && !strings.Contains(strings.Join(args, " "), "--wasm-compiler=ion") {
				t.Fatal(args)
			}
			if id == "jsc" && !strings.Contains(strings.Join(args, " "), "--thresholdForOMGOptimizeAfterWarmUp=1") {
				t.Fatal(args)
			}
			if id == "jsc" && !strings.Contains(strings.Join(args, " "), "runtime-version=WebKitGTK/2.52.6") {
				t.Fatal(args)
			}
		})
	}
	for _, id := range []string{"wasmi", "wavm", "wasm3", "wasmedge"} {
		args, ok, err := extraRuntimeCommand(root, id)
		if err != nil || !ok || len(args) != 1 || !strings.Contains(args[0], "adapter-"+id) {
			t.Fatal(args, ok, err)
		}
	}
	if _, ok, _ := extraRuntimeCommand(root, "unknown"); ok {
		t.Fatal("unknown runtime accepted")
	}
}

func TestResolveV8UsesPinnedNodeBinary(t *testing.T) {
	root := t.TempDir()
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("release-pinned node"), 0755); err != nil {
		t.Fatal(err)
	}
	adapterDir := filepath.Join(root, "adapters", "v8")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"adapter.mjs", "floats.mjs", "profiling.mjs", "compiler-mode.mjs", "harness.mjs", "wasi-readonly.mjs"} {
		if err := os.WriteFile(filepath.Join(adapterDir, name), []byte("// fixture\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("WASMBENCH_NODE", node)
	runtimes, err := ResolveRuntimes(root, []string{"v8-optimizing-only"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimes) != 1 || runtimes[0].Command[0] != node {
		t.Fatalf("V8 did not use the pinned Node executable: %#v", runtimes)
	}
	if !strings.Contains(strings.Join(runtimes[0].Command, " "), "--no-liftoff") {
		t.Fatalf("V8 optimizing-only flags were lost: %#v", runtimes[0].Command)
	}
	t.Setenv("WASMBENCH_V8_COMPILER_MODE", "optimizing-only")
	runtimes, err = ResolveRuntimes(root, []string{"v8"})
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(runtimes[0].Command, " ")
	for _, flag := range []string{"--no-liftoff", "--no-wasm-tier-up", "--no-wasm-lazy-compilation", "--no-wasm-native-module-cache", "--compiler-mode=optimizing-only"} {
		if !strings.Contains(command, flag) {
			t.Fatalf("controlled base V8 lost %s: %s", flag, command)
		}
	}
	if runtimes[0].ID != "v8" {
		t.Fatal("changed requested runtime identity")
	}
	t.Setenv("WASMBENCH_V8_COMPILER_MODE", "invalid")
	if _, err = ResolveRuntimes(root, []string{"v8"}); err == nil {
		t.Fatal("accepted invalid tier policy")
	}
}

func TestExtraShellProbe(t *testing.T) {
	good := []byte("{\"version\":1,\"id\":1,\"status\":\"ok\",\"description\":{\"runtime\":\"v8\"}}\n{\"version\":1,\"id\":2,\"status\":\"ok\"}\n")
	if err := validateExtraProbe(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", string(good) + "banner\n", strings.Replace(string(good), "\"id\":1", "\"id\":3", 1), strings.Replace(string(good), "\"ok\"", "\"unsupported\"", 1), "not json\nnot json"} {
		if validateExtraProbe([]byte(bad)) == nil {
			t.Fatal("accepted", bad)
		}
	}
}

func TestResolveJSCVersionArgument(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "adapters", "js-shell", "adapter.js")
	if err := os.MkdirAll(filepath.Dir(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("// test adapter\n"), 0644); err != nil {
		t.Fatal(err)
	}
	shell := filepath.Join(root, "adapter-jsc")
	if err := os.WriteFile(shell, []byte("test engine"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASMBENCH_JSC", shell)
	t.Setenv("WASMBENCH_JSC_VERSION", "WebKitGTK/2.52.6")
	runtimes, err := ResolveRuntimes(root, []string{"jsc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimes) != 1 || runtimes[0].ID != "jsc" || !strings.Contains(strings.Join(runtimes[0].Command, " "), "runtime-version=WebKitGTK/2.52.6") {
		t.Fatalf("unexpected JSC runtime configuration: %#v", runtimes)
	}
}

func TestResolveJSCHostUsesTheEmbeddingProtocolAndExactVersion(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "adapters", "js-shell"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "adapters", "js-shell", "adapter.js"), []byte("// test adapter\n"), 0644); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(root, "adapter-jsc")
	if err := os.WriteFile(host, []byte("test engine"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WASMBENCH_JSC_HOST", host)
	t.Setenv("WASMBENCH_JSC_VERSION", "WebKitGTK/2.52.6")
	args, supported, err := extraRuntimeCommand(root, "jsc")
	if err != nil || !supported {
		t.Fatal(args, supported, err)
	}
	joined := strings.Join(args, " ")
	for _, token := range []string{host, filepath.Join(root, "adapters", "js-shell", "adapter.js"), "runtime=jsc", "tier-mode=omg-eager", "runtime-version=WebKitGTK/2.52.6", "binary-sha256="} {
		if !strings.Contains(joined, token) {
			t.Fatalf("JSC embedding command lacks %q: %s", token, joined)
		}
	}
	if strings.Contains(joined, "--thresholdForOMG") {
		t.Fatalf("embedding host received standalone shell flags: %s", joined)
	}
}
