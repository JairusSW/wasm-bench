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
			args, ok, err := extraRuntimeCommand(root, id)
			if err != nil || !ok {
				t.Fatal(args, ok, err)
			}
			if args[0] != shell || !strings.Contains(strings.Join(args, " "), "runtime="+id) || !strings.Contains(strings.Join(args, " "), "binary-sha256=") {
				t.Fatal(args)
			}
			if id == "deno" && !strings.Contains(strings.Join(args, " "), "--no-prompt --allow-read") {
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
