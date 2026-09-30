package publish

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/wasmbench/wasmbench/experiment"
)

// Keep the existing evidence verifiers usable for legacy exports. Newly marked
// datasets require their builder receipt; removing it cannot downgrade them.
// This intentionally does not call VerifyAnyReport: native generators verify
// themselves after sealing, so dispatching regeneration here would recurse.
func verifyNativeBuilder(root, kind, version string) error {
	if version == "" {
		for _, name := range []string{"builder.json", "builder"} {
			if _, err := os.Stat(filepath.Join(root, name)); err == nil {
				return fmt.Errorf("unrecorded native report builder archive")
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	if version != FamilyBuilderVersion {
		return fmt.Errorf("unsupported native builder archive version")
	}
	r, err := loadAnyBuilderReceipt(root)
	if err != nil {
		return err
	}
	if r.Version != version || r.Kind != kind {
		return fmt.Errorf("native report builder family differs from dataset")
	}
	return nil
}

// Rebuild derived records/images/HTML using raw evidence, while retaining LLVM
// outputs as sealed diagnostics. This is NOT independent redisassembly. No tool
// path or command stored in the input is ever executed during verification.
func regenerateNativeDisassembly(root, out string) error {
	if err := VerifyNativeCode(root); err != nil {
		return err
	}
	var saved NativeExport
	if err := experiment.ReadJSON(filepath.Join(root, "native-code.json"), &saved); err != nil {
		return err
	}
	if saved.Version != "native-image-disassembly-v1" && saved.Version != "native-image-disassembly-v2" {
		return fmt.Errorf("expected native disassembly evidence")
	}
	return exportNativeCode(filepath.Join(root, "raw"), out, func(dir string, derived *NativeExport) error {
		derived.Version = saved.Version
		derived.ToolTimeoutNS = saved.ToolTimeoutNS
		derived.ToolOutputLimitBytes = saved.ToolOutputLimitBytes
		derived.Tools = saved.Tools
		derived.Interpretation = nativeDisassemblyInterpretation
		for i := range derived.Records {
			d := saved.Records[i].Disassembly
			if d == nil {
				continue
			}
			// VerifyNativeCode above checked all these paths against canonical
			// image indices and full function mappings before any output writes.
			paths := []string{d.Object, d.Text, d.ObjcopyLog, d.ObjdumpLog}
			for _, f := range d.Functions {
				paths = append(paths, f.Text, f.ObjdumpLog)
			}
			for _, path := range paths {
				if err := experiment.CopyExclusive(filepath.Join(root, path), filepath.Join(dir, path)); err != nil {
					return err
				}
			}
			derived.Records[i].Disassembly = d
		}
		return nil
	})
}
