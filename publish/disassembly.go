package publish

import (
	"bytes"
	"context"
	"debug/elf"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

type NativeTool struct {
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Version string `json:"version"`
}

type NativeDisassembly struct {
	Status      string                `json:"status"`
	Object      string                `json:"object"`
	Text        string                `json:"text"`
	ObjcopyArgs []string              `json:"objcopy_args"`
	ObjdumpArgs []string              `json:"objdump_args"`
	ObjcopyLog  string                `json:"objcopy_log"`
	ObjdumpLog  string                `json:"objdump_log"`
	Functions   []FunctionDisassembly `json:"functions,omitempty"`
}

type FunctionDisassembly struct {
	Function    protocol.CodeFunction `json:"function"`
	Text        string                `json:"text"`
	ObjdumpLog  string                `json:"objdump_log"`
	ObjdumpArgs []string              `json:"objdump_args"`
	Listing     string                `json:"listing"`
}

// Range decoding preserves text-relative addresses and PC-relative operands by
// using the original complete text object, rather than rebasing sliced bytes.
func functionDisassembly(object, image string, index int, f protocol.CodeFunction) FunctionDisassembly {
	path := fmt.Sprintf("%s.function-%06d", image, index)
	return FunctionDisassembly{Function: f, Text: path + ".asm", ObjdumpLog: path + ".objdump.log", ObjdumpArgs: []string{"--disassemble", "--disassemble-zeroes", "--section=.text", fmt.Sprintf("--start-address=%d", f.Offset), fmt.Sprintf("--stop-address=%d", f.Offset+f.Length), object}}
}

// DisassembleNativeCode wraps images in synthetic ELF objects, independently
// verifies byte identity, and disassembles offline. It never executes the images.
func DisassembleNativeCode(ctx context.Context, root, out, objcopy, objdump string, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("tool timeout must be positive")
	}
	if err := reportOutsideInputs([]string{root}, out); err != nil {
		return err
	}
	copyTool, err := identifyNativeTool(ctx, objcopy, timeout)
	if err != nil {
		return err
	}
	dumpTool, err := identifyNativeTool(ctx, objdump, timeout)
	if err != nil {
		return err
	}
	return disassembleNativeCodeWithTools(ctx, root, out, copyTool, dumpTool, timeout)
}

// Replay supplies recorded identities directly. Every invocation checks these
// exact hashes before and after execution rather than silently identifying a
// replacement tool after the replay preflight.
func disassembleNativeCodeWithTools(ctx context.Context, root, out string, copyTool, dumpTool NativeTool, timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("tool timeout must be positive")
	}
	return exportNativeCode(root, out, func(dir string, r *NativeExport) error {
		r.Version = "native-image-disassembly-v2"
		r.ToolTimeoutNS = int64(timeout)
		r.ToolOutputLimitBytes = 64 << 20
		r.Tools = []NativeTool{copyTool, dumpTool}
		r.Interpretation = nativeDisassemblyInterpretation
		for i := range r.Records {
			record := &r.Records[i]
			if record.Status != "available" {
				continue
			}
			format, machine, err := nativeELFTarget(record.Image.Architecture)
			if err != nil {
				return err
			}
			d := &NativeDisassembly{Status: "linear_mixed_image", Object: record.Path + ".o", Text: record.Path + ".asm", ObjcopyLog: record.Path + ".objcopy.log", ObjdumpLog: record.Path + ".objdump.log"}
			d.ObjcopyArgs = []string{"--input-target=binary", "--output-target=" + format, "--rename-section=.data=.text,alloc,load,readonly,code,contents", record.Path, d.Object}
			stdout, stderr, err := runNativeTool(ctx, copyTool, dir, d.ObjcopyArgs, timeout)
			if logErr := writeDiagnostic(filepath.Join(dir, d.ObjcopyLog), append(stdout, stderr...)); logErr != nil {
				return logErr
			}
			if err != nil {
				return fmt.Errorf("objcopy %s: %w: %s", record.Trial, err, stderr)
			}
			if err = verifyNativeObject(filepath.Join(dir, d.Object), filepath.Join(dir, record.Path), machine); err != nil {
				return err
			}
			if err = os.Chmod(filepath.Join(dir, d.Object), 0644); err != nil {
				return err
			}
			d.ObjdumpArgs = []string{"--disassemble", "--disassemble-zeroes", "--section=.text", d.Object}
			stdout, stderr, err = runNativeTool(ctx, dumpTool, dir, d.ObjdumpArgs, timeout)
			if logErr := writeDiagnostic(filepath.Join(dir, d.Text), stdout); logErr != nil {
				return logErr
			}
			if logErr := writeDiagnostic(filepath.Join(dir, d.ObjdumpLog), stderr); logErr != nil {
				return logErr
			}
			if err != nil {
				return fmt.Errorf("objdump %s: %w: %s", record.Trial, err, stderr)
			}
			if len(stdout) == 0 {
				return fmt.Errorf("objdump produced no disassembly")
			}
			var listingBytes int
			for j, f := range record.Image.Functions {
				entry := functionDisassembly(d.Object, record.Path, j, f)
				text, log, err := runNativeTool(ctx, dumpTool, dir, entry.ObjdumpArgs, timeout)
				if logErr := writeDiagnostic(filepath.Join(dir, entry.ObjdumpLog), log); logErr != nil {
					return logErr
				}
				if err != nil {
					return fmt.Errorf("objdump %s function %d: %w: %s", record.Trial, f.WasmIndex, err, log)
				}
				listingBytes += len(text)
				if len(text) == 0 || listingBytes > 64<<20 {
					return fmt.Errorf("function listings empty or exceed 64 MiB per-image budget")
				}
				if err = writeDiagnostic(filepath.Join(dir, entry.Text), text); err != nil {
					return err
				}
				entry.Listing = string(text)
				d.Functions = append(d.Functions, entry)
			}
			record.Disassembly = d
		}
		return nil
	})
}

func nativeELFTarget(arch string) (string, elf.Machine, error) {
	switch arch {
	case "arm64":
		return "elf64-littleaarch64", elf.EM_AARCH64, nil
	case "amd64":
		return "elf64-x86-64", elf.EM_X86_64, nil
	}
	return "", 0, fmt.Errorf("unsupported native architecture %q", arch)
}

func verifyNativeObject(object, raw string, machine elf.Machine) error {
	f, err := elf.Open(object)
	if err != nil {
		return err
	}
	defer f.Close()
	if f.Type != elf.ET_REL || f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != machine {
		return fmt.Errorf("synthetic native object header mismatch")
	}
	s := f.Section(".text")
	if s == nil || s.Addr != 0 || s.Type != elf.SHT_PROGBITS {
		return fmt.Errorf("synthetic native object text section mismatch")
	}
	want, err := os.ReadFile(raw)
	if err != nil {
		return err
	}
	if s.Size != uint64(len(want)) {
		return fmt.Errorf("synthetic native object size mismatch")
	}
	got, err := s.Data()
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("synthetic native object changed image bytes")
	}
	return nil
}

func identifyNativeTool(ctx context.Context, path string, timeout time.Duration) (NativeTool, error) {
	var t NativeTool
	p, err := exec.LookPath(path)
	if err != nil {
		return t, err
	}
	t.Path, err = filepath.Abs(p)
	if err != nil {
		return t, err
	}
	t.SHA256, err = experiment.DigestFile(t.Path)
	if err != nil {
		return t, err
	}
	stdout, stderr, err := runNativeTool(ctx, t, "", []string{"--version"}, timeout)
	if err != nil {
		return t, fmt.Errorf("tool version: %w: %s", err, stderr)
	}
	t.Version = string(stdout)
	if t.Version == "" {
		return t, fmt.Errorf("tool did not report a version")
	}
	return t, nil
}

type nativeOutput struct {
	data     bytes.Buffer
	overflow bool
}

func (b *nativeOutput) Bytes() []byte { return b.data.Bytes() }

func (b *nativeOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (64 << 20) - b.data.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, err := b.data.Write(p)
	return n, err
}

func runNativeTool(ctx context.Context, t NativeTool, dir string, args []string, timeout time.Duration) ([]byte, []byte, error) {
	before, err := experiment.DigestFile(t.Path)
	if err != nil {
		return nil, nil, err
	}
	if before != t.SHA256 {
		return nil, nil, fmt.Errorf("native tool changed before execution")
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, t.Path, args...)
	cmd.Dir = dir
	cmd.Env = []string{"LC_ALL=C", "LANG=C"}
	cmd.WaitDelay = time.Second
	var stdout, stderr nativeOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if stdout.overflow || stderr.overflow {
		err = fmt.Errorf("tool output exceeds 64 MiB limit; disassembly is incomplete")
	}
	after, hashErr := experiment.DigestFile(t.Path)
	if hashErr != nil {
		err = hashErr
	} else if after != before {
		err = fmt.Errorf("native tool changed during execution")
	}
	return stdout.Bytes(), stderr.Bytes(), err
}

func writeDiagnostic(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
