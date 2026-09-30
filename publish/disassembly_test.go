package publish

import (
	"context"
	"debug/elf"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wasmbench/wasmbench/corpus"
	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
	"reflect"
)

func TestNativeTargets(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		format, machine, err := nativeELFTarget(arch)
		if err != nil || format == "" || machine == elf.EM_NONE {
			t.Fatal(format, machine, err)
		}
	}
	if _, _, err := nativeELFTarget("unknown"); err == nil {
		t.Fatal("unknown target accepted")
	}
}

func TestFunctionDisassemblyKeepsOriginalRangeAddresses(t *testing.T) {
	f := protocol.CodeFunction{WasmIndex: 42, Offset: 4096, Length: 12, Tier: "winch"}
	d := functionDisassembly("image.o", "image.bin", 3, f)
	want := []string{"--disassemble", "--disassemble-zeroes", "--section=.text", "--start-address=4096", "--stop-address=4108", "image.o"}
	if !reflect.DeepEqual(d.ObjdumpArgs, want) || d.Function != f || d.Text != "image.bin.function-000003.asm" {
		t.Fatal(d)
	}
}

func TestNativeOutputLimit(t *testing.T) {
	var output nativeOutput
	// io.Copy must not bypass the limit through an embedded ReaderFrom method.
	n, err := io.Copy(&output, strings.NewReader(strings.Repeat("x", (64<<20)+1)))
	if err != nil || n != (64<<20)+1 || len(output.Bytes()) != 64<<20 || !output.overflow {
		t.Fatal(n, err, output.overflow, len(output.Bytes()))
	}
}

func TestNativeToolRejectsChangedExecutable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tool")
	if err := writeDiagnostic(path, []byte("not executable")); err != nil {
		t.Fatal(err)
	}
	_, _, err := runNativeTool(context.Background(), NativeTool{Path: path, SHA256: "wrong"}, "", nil, time.Second)
	if err == nil || !strings.Contains(err.Error(), "changed before") {
		t.Fatal(err)
	}
}

func TestNativeToolDeadline(t *testing.T) {
	path, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep unavailable")
	}
	digest, err := experiment.DigestFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runNativeTool(context.Background(), NativeTool{Path: path, SHA256: digest}, "", []string{"30"}, 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}

func TestDisassemblyInvalidRequestDoesNotWrite(t *testing.T) {
	root := nativeBundle(t, "code")
	out := filepath.Join(root, "nested")
	if err := DisassembleNativeCode(context.Background(), root, out, "missing", "missing", time.Second); err == nil {
		t.Fatal("nested output accepted")
	}
	if err := DisassembleNativeCode(context.Background(), root, out, "missing", "missing", 0); err == nil {
		t.Fatal("invalid timeout accepted")
	}
	if err := experiment.Verify(root); err != nil {
		t.Fatal(err)
	}
}

func TestLLVMNativeDisassembly(t *testing.T) {
	if os.Getenv("WASMBENCH_NATIVE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_NATIVE_LLVM_TEST=1 for installed LLVM integration")
	}
	copyPath := os.Getenv("WASMBENCH_LLVM_OBJCOPY")
	if copyPath == "" {
		copyPath = "/opt/homebrew/opt/llvm/bin/llvm-objcopy"
	}
	dumpPath := os.Getenv("WASMBENCH_LLVM_OBJDUMP")
	if dumpPath == "" {
		dumpPath = "/opt/homebrew/opt/llvm/bin/llvm-objdump"
	}
	ctx := context.Background()
	copyTool, err := identifyNativeTool(ctx, copyPath, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dumpTool, err := identifyNativeTool(ctx, dumpPath, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		arch string
		data []byte
	}{{"amd64", []byte{0xc3}}, {"arm64", []byte{0xc0, 0x03, 0x5f, 0xd6}}} {
		t.Run(tc.arch, func(t *testing.T) {
			dir := t.TempDir()
			raw := filepath.Join(dir, "image.bin")
			object := filepath.Join(dir, "image.o")
			if err := writeDiagnostic(raw, tc.data); err != nil {
				t.Fatal(err)
			}
			format, machine, _ := nativeELFTarget(tc.arch)
			_, log, err := runNativeTool(ctx, copyTool, dir, []string{"-I", "binary", "-O", format, "--rename-section=.data=.text,alloc,load,readonly,code,contents", "image.bin", "image.o"}, 10*time.Second)
			if err != nil {
				t.Fatalf("%v %s", err, log)
			}
			if err = verifyNativeObject(object, raw, machine); err != nil {
				t.Fatal(err)
			}
			if err = verifyNativeObject(object, raw, elf.EM_NONE); err == nil {
				t.Fatal("wrong machine accepted")
			}
			text, log, err := runNativeTool(ctx, dumpTool, dir, []string{"-d", "--disassemble-zeroes", "--section=.text", "image.o"}, 10*time.Second)
			if err != nil || !strings.Contains(string(text), "ret") {
				t.Fatalf("%v %s %s", err, text, log)
			}
		})
	}
	root := nativeBundle(t, "code")
	out := filepath.Join(t.TempDir(), "export")
	if err = DisassembleNativeCode(ctx, root, out, copyPath, dumpPath, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if err = experiment.Verify(out); err != nil {
		t.Fatal(err)
	}
	var r NativeExport
	if err = experiment.ReadJSON(filepath.Join(out, "native-code.json"), &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Tools) != 2 || r.Records[0].Disassembly == nil || r.Records[1].Disassembly != nil || r.Version != "native-image-disassembly-v2" {
		t.Fatal(r)
	}
	if err = DisassembleNativeCode(ctx, root, out, copyPath, dumpPath, 10*time.Second); err == nil {
		t.Fatal("overwrote output")
	}
	if err = experiment.Verify(out); err != nil {
		t.Fatal(err)
	}
	badTool := filepath.Join(t.TempDir(), "objdump-failure")
	if err = writeDiagnostic(badTool, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo test-tool; exit 0; fi\necho deliberate-disassembler-failure >&2\nexit 7\n")); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(badTool, 0700); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(t.TempDir(), "failed-export")
	if err = DisassembleNativeCode(ctx, root, failed, copyPath, badTool, 10*time.Second); err == nil {
		t.Fatal("tool failure accepted")
	}
	if _, err = os.Stat(filepath.Join(failed, "checksums.json")); !os.IsNotExist(err) {
		t.Fatal("sealed failed export", err)
	}
	log, err := os.ReadFile(filepath.Join(failed, "image-000000.bin.objdump.log"))
	if err != nil || !strings.Contains(string(log), "deliberate-disassembler-failure") {
		t.Fatal(string(log), err)
	}
	if err = experiment.Verify(root); err != nil {
		t.Fatal("source modified", err)
	}
}

func TestLLVMFunctionDisassemblyAndForgedMapping(t *testing.T) {
	if os.Getenv("WASMBENCH_NATIVE_LLVM_TEST") != "1" {
		t.Skip("set WASMBENCH_NATIVE_LLVM_TEST=1 for installed LLVM integration")
	}
	copyPath := os.Getenv("WASMBENCH_LLVM_OBJCOPY")
	if copyPath == "" {
		copyPath = "/opt/homebrew/opt/llvm/bin/llvm-objcopy"
	}
	dumpPath := os.Getenv("WASMBENCH_LLVM_OBJDUMP")
	if dumpPath == "" {
		dumpPath = "/opt/homebrew/opt/llvm/bin/llvm-objdump"
	}
	replace := func(path string, v any) {
		t.Helper()
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	reseal := func(root string) {
		t.Helper()
		if err := os.Remove(filepath.Join(root, "checksums.json")); err != nil {
			t.Fatal(err)
		}
		if err := experiment.Seal(root); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		arch   string
		data   []byte
		length uint64
	}{{"amd64", []byte{0x90, 0xc3, 0x90, 0xc3}, 2}, {"arm64", []byte{0xc0, 0x03, 0x5f, 0xd6, 0xc0, 0x03, 0x5f, 0xd6}, 4}} {
		t.Run(tc.arch, func(t *testing.T) {
			root := nativeBundle(t, "code")
			path := filepath.Join(root, "trials", "a.json")
			var trial experiment.Trial
			if err := experiment.ReadJSON(path, &trial); err != nil {
				t.Fatal(err)
			}
			trial.CodeImage.Version = 2
			trial.CodeImage.Architecture = tc.arch
			trial.CodeImage.Backend = "cranelift"
			trial.CodeImage.FunctionAttribution = "engine_reported"
			trial.CodeImage.Data = tc.data
			trial.CodeImage.SHA256 = corpus.Hash(tc.data)
			trial.CodeImage.Functions = []protocol.CodeFunction{{WasmIndex: 7, Length: tc.length, Tier: "cranelift"}, {WasmIndex: 8, Offset: tc.length, Length: tc.length, Tier: "cranelift"}}
			replace(path, trial)
			reseal(root)
			out := filepath.Join(t.TempDir(), "export")
			if err := DisassembleNativeCode(context.Background(), root, out, copyPath, dumpPath, 10*time.Second); err != nil {
				t.Fatal(err)
			}
			var r NativeExport
			if err := experiment.ReadJSON(filepath.Join(out, "native-code.json"), &r); err != nil {
				t.Fatal(err)
			}
			listings := r.Records[0].Disassembly.Functions
			if len(listings) != 2 {
				t.Fatal("missing functions")
			}
			for _, listing := range listings {
				text, err := os.ReadFile(filepath.Join(out, listing.Text))
				if err != nil || strings.Count(string(text), "\tret") != 1 {
					t.Fatalf("range listing crossed function boundary: %v %s", err, text)
				}
			}
			original, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			for _, mode := range []string{"arguments", "identity", "coverage", "listing"} {
				var changed NativeExport
				if err := json.Unmarshal(original, &changed); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "arguments":
					changed.Records[0].Disassembly.Functions[1].ObjdumpArgs[3] = "--start-address=0"
				case "identity":
					changed.Records[0].Disassembly.Functions[0].Function.WasmIndex = 999
				case "coverage":
					changed.Records[0].Disassembly.Functions = changed.Records[0].Disassembly.Functions[:1]
				case "listing":
					changed.Records[0].Disassembly.Functions[0].Listing = "fabricated"
				}
				replace(filepath.Join(out, "native-code.json"), changed)
				reseal(out)
				if VerifyNativeCode(out) == nil {
					t.Fatalf("resealed %s forgery accepted", mode)
				}
			}
		})
	}
}
