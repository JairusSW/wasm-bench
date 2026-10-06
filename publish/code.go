package publish

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

//go:embed code.html code_functions.html
var codeAssets embed.FS

const nativeExportInterpretation = "Raw native images include wrappers and embedded data. Version 2 records retain engine-reported guest function ranges, Wasm indices and fixed backend generation zero; ranges may include constants and padding, not just instructions. Version 1 images have no function attribution. Unattributed text is not automatically trampoline size. These are retained compilation snapshots, not creation/retirement logs. Original relocations and instruction-only sizes remain unavailable. Metadata data fields are omitted here; path identifies exact bytes. The complete source evidence is in raw/. Images are data, not executable files."
const nativeDisassemblyInterpretation = "Synthetic ELF wrappers preserve the raw mixed native image byte-for-byte at section offset zero. Linear disassembly may decode embedded data as instructions; synthetic binary symbols are not Wasm function boundaries. Version 2 images retain engine-reported function ranges and backend generation zero. Per-function listings restrict decoding to those ranges in the complete text object, preserving image-relative addresses; version 1 images have no function attribution. Range listings may decode function-local constants/padding and do not establish instruction-only sizes. No original object, relocations, tier lifetime events or compiler counters are inferred. Tool executable hashes, versions and argv are recorded; dynamic libraries are not pinned. The complete source evidence is in raw/."

const nativeFunctionDisassemblyInterpretation = nativeDisassemblyInterpretation + " Version 3 collects function ranges only; whole-image linear disassembly is not collected. Tool output remains bounded at 64 MiB per invocation and summed function listings at 128 MiB per image."

func functionListingLimit(r NativeExport) int {
	if r.Version == "native-image-disassembly-v3" {
		return r.FunctionListingLimitBytes
	}
	return 64 << 20
}

type NativeExportRecord struct {
	Expansion   *NativeExpansion    `json:"expansion,omitempty"`
	Disassembly *NativeDisassembly  `json:"disassembly,omitempty"`
	Trial       string              `json:"trial"`
	Runtime     string              `json:"runtime_configuration"`
	Workload    string              `json:"workload"`
	Block       int                 `json:"block"`
	TrialStatus string              `json:"trial_status"`
	Status      string              `json:"export_status"`
	Reason      string              `json:"reason,omitempty"`
	Path        string              `json:"path,omitempty"`
	Bytes       *int                `json:"bytes"`
	Image       *protocol.CodeImage `json:"image,omitempty"`
}

const nativeMaterializedExportInterpretation = nativeExportInterpretation + " Version 3 adds complete defined-function coverage at a documented synchronous compile return, checked against independent input analysis. Its compile timer is code-pass diagnostic evidence, not headline latency. It does not add lifecycle or reclamation observations."

type NativeExport struct {
	FunctionListingLimitBytes int                  `json:"function_listing_limit_bytes,omitempty"`
	BuilderArchiveVersion     string               `json:"builder_archive_version,omitempty"`
	RendererSHA256            string               `json:"renderer_sha256"`
	ToolTimeoutNS             int64                `json:"tool_timeout_ns,omitempty"`
	ToolOutputLimitBytes      int                  `json:"tool_output_limit_bytes,omitempty"`
	Tools                     []NativeTool         `json:"tools,omitempty"`
	Version                   string               `json:"version"`
	Run                       string               `json:"run"`
	SourceChecksumsSHA256     string               `json:"source_checksums_sha256"`
	Interpretation            string               `json:"interpretation"`
	Records                   []NativeExportRecord `json:"records"`
}

// ExportNativeCode is offline: it only decodes already-verified binary evidence.
// It does not execute images, invoke disassemblers, or fabricate function ranges.
func ExportNativeCode(root, out string) error {
	return exportNativeCode(root, out, nil)
}

func exportNativeCode(root, out string, finish func(string, *NativeExport) error) error {
	if err := reportOutsideInputs([]string{root}, out); err != nil {
		return err
	}
	b, err := experiment.Load(root)
	if err != nil {
		return err
	}
	if b.Manifest.Lock.Options.Profile != "code" {
		return fmt.Errorf("native export requires a code-profile bundle")
	}
	digest, err := experiment.DigestFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	raw := filepath.Join(out, "raw")
	if err = os.CopyFS(raw, os.DirFS(root)); err != nil {
		return err
	}
	// Use the verified copy as the sole extraction source, including the manifest.
	b, err = experiment.Load(raw)
	if err != nil {
		return err
	}
	copied, err := experiment.DigestFile(filepath.Join(raw, "checksums.json"))
	if err != nil {
		return err
	}
	if copied != digest {
		return fmt.Errorf("native export evidence changed while copying")
	}
	rendererHash, err := codeRendererHash()
	if err != nil {
		return err
	}
	r := NativeExport{BuilderArchiveVersion: FamilyBuilderVersion, Version: "native-image-export-v1", Run: b.Manifest.ID, SourceChecksumsSHA256: digest, RendererSHA256: rendererHash, Interpretation: nativeMaterializedExportInterpretation, Records: []NativeExportRecord{}}
	sources, err := nativeBodySources(raw, b)
	if err != nil {
		return err
	}
	for i, t := range b.Trials {
		record := nativeRecord(t, i)
		if record.Image != nil {
			record.Expansion, err = nativeExpansion(record.Image, uint64(*record.Bytes), sources)
			if err != nil {
				return err
			}
		}
		if record.Status == "available" {
			f, err := os.OpenFile(filepath.Join(out, record.Path), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
			if err != nil {
				return err
			}
			_, writeErr := f.Write(t.CodeImage.Data)
			if err = errors.Join(writeErr, f.Close()); err != nil {
				return err
			}
		}
		r.Records = append(r.Records, record)
	}
	if finish != nil {
		if err = finish(out, &r); err != nil {
			return err
		}
	}
	r.RendererSHA256, err = codeRendererHashFor(r.Version)
	if err != nil {
		return err
	}
	if err = experiment.WriteJSON(filepath.Join(out, "native-code.json"), r); err != nil {
		return err
	}
	html, err := renderCodeHTML(r)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(out, "index.html"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err = f.Write(html); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	kind := "native-export"
	if r.Version != "native-image-export-v1" {
		kind = "native-disassembly"
	}
	if err := sealFamilyReport(out, kind); err != nil {
		return err
	}
	return VerifyNativeCode(out)
}

func nativeRecord(t experiment.Trial, i int) NativeExportRecord {
	record := NativeExportRecord{Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Block: t.Block, TrialStatus: t.Status, Status: "not_recorded", Reason: "adapter did not record a native image"}
	if t.Block < 0 {
		record.Status, record.Reason = "not_collected", "sacrificial correctness trial"
	} else if t.Status != "ok" {
		record.Status, record.Reason = "unavailable", t.Reason
	} else if t.CodeImage != nil {
		img := *t.CodeImage
		size := len(img.Data)
		record.Status, record.Reason = "available", ""
		record.Path = fmt.Sprintf("image-%06d.bin", i)
		record.Bytes = &size
		img.Data = nil
		record.Image = &img
	} else {
		var guestCode *protocol.Observation
		for _, o := range t.Observations {
			if o.Metric == "native.code_export" && o.Status != "available" {
				record.Status, record.Reason = o.Status, o.Reason
				return record
			}
			if o.Metric == "native.guest_code" && o.Status != "available" {
				copy := o
				guestCode = &copy
			}
		}
		if guestCode != nil {
			record.Status, record.Reason = guestCode.Status, guestCode.Reason
		}
	}
	return record
}

// VerifyNativeCode checks the derived code page against its sealed source and
// exact extracted image bytes. Disassembly text remains tool-produced diagnostic
// evidence, protected by the outer seal but not independently re-disassembled.
func VerifyNativeCode(root string) error {
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var recorded NativeExport
	if err := experiment.ReadJSON(filepath.Join(root, "native-code.json"), &recorded); err != nil {
		return err
	}
	if recorded.Version != "native-image-export-v1" && recorded.Version != "native-image-disassembly-v1" && recorded.Version != "native-image-disassembly-v2" && recorded.Version != "native-image-disassembly-v3" {
		return fmt.Errorf("unsupported native export version %q", recorded.Version)
	}
	rendererHash, err := codeRendererHashFor(recorded.Version)
	if err != nil {
		return err
	}
	if recorded.RendererSHA256 != rendererHash {
		return fmt.Errorf("native code renderer digest differs")
	}
	if recorded.Version == "native-image-export-v1" {
		if recorded.Interpretation != nativeMaterializedExportInterpretation || len(recorded.Tools) != 0 || recorded.ToolTimeoutNS != 0 || recorded.ToolOutputLimitBytes != 0 || recorded.FunctionListingLimitBytes != 0 {
			return fmt.Errorf("native image export policy differs")
		}
	} else if recorded.Version == "native-image-disassembly-v3" {
		if recorded.Interpretation != nativeFunctionDisassemblyInterpretation || recorded.FunctionListingLimitBytes != 128<<20 || len(recorded.Tools) != 2 || recorded.ToolTimeoutNS <= 0 || recorded.ToolOutputLimitBytes != 64<<20 {
			return fmt.Errorf("native function-only disassembly policy differs")
		}
	} else if recorded.FunctionListingLimitBytes != 0 || recorded.Interpretation != nativeDisassemblyInterpretation || len(recorded.Tools) != 2 || recorded.ToolTimeoutNS <= 0 || recorded.ToolOutputLimitBytes != 64<<20 {
		return fmt.Errorf("native disassembly policy differs")
	}
	for _, tool := range recorded.Tools {
		decoded, err := hex.DecodeString(tool.SHA256)
		if !filepath.IsAbs(tool.Path) || len(decoded) != 32 || err != nil || tool.Version == "" {
			return fmt.Errorf("incomplete native disassembly tool identity")
		}
	}
	input := filepath.Join(root, "raw")
	b, err := experiment.Load(input)
	if err != nil {
		return err
	}
	digest, err := experiment.DigestFile(filepath.Join(input, "checksums.json"))
	if err != nil {
		return err
	}
	if b.Manifest.Lock.Options.Profile != "code" || recorded.Run != b.Manifest.ID || recorded.SourceChecksumsSHA256 != digest || len(recorded.Records) != len(b.Trials) {
		return fmt.Errorf("native export source identity or trial count differs")
	}
	sources, err := nativeBodySources(input, b)
	if err != nil {
		return err
	}
	for i, trial := range b.Trials {
		got, want := recorded.Records[i], nativeRecord(trial, i)
		if want.Image != nil {
			want.Expansion, err = nativeExpansion(want.Image, uint64(*want.Bytes), sources)
			if err != nil {
				return err
			}
		}
		if got.Disassembly != nil {
			if (recorded.Version != "native-image-disassembly-v1" && recorded.Version != "native-image-disassembly-v2" && recorded.Version != "native-image-disassembly-v3") || got.Status != "available" {
				return fmt.Errorf("unexpected disassembly for %s", got.Trial)
			}
			d := got.Disassembly
			validPaths := d.Status == "linear_mixed_image" && d.Text == got.Path+".asm" && d.ObjdumpLog == got.Path+".objdump.log"
			if recorded.Version == "native-image-disassembly-v3" {
				validPaths = d.Status == "function_ranges" && d.Text == "" && d.ObjdumpLog == "" && len(got.Image.Functions) > 0
			}
			if !validPaths || d.Object != got.Path+".o" || d.ObjcopyLog != got.Path+".objcopy.log" {
				return fmt.Errorf("unexpected disassembly paths for %s", got.Trial)
			}
			format, _, err := nativeELFTarget(got.Image.Architecture)
			if err != nil {
				return err
			}
			copyArgs := []string{"--input-target=binary", "--output-target=" + format, "--rename-section=.data=.text,alloc,load,readonly,code,contents", got.Path, d.Object}
			dumpArgs := []string{"--disassemble", "--disassemble-zeroes", "--section=.text", d.Object}
			if recorded.Version == "native-image-disassembly-v3" {
				dumpArgs = nil
			}
			if !reflect.DeepEqual(d.ObjcopyArgs, copyArgs) || !reflect.DeepEqual(d.ObjdumpArgs, dumpArgs) {
				return fmt.Errorf("unexpected disassembly tool arguments for %s", got.Trial)
			}
			if recorded.Version == "native-image-disassembly-v2" || recorded.Version == "native-image-disassembly-v3" {
				if len(d.Functions) != len(got.Image.Functions) {
					return fmt.Errorf("native function listing coverage mismatch")
				}
				var listingBytes int64
				for j, f := range got.Image.Functions {
					expected := functionDisassembly(d.Object, got.Path, j, f)
					expected.Listing = d.Functions[j].Listing
					if !reflect.DeepEqual(d.Functions[j], expected) {
						return fmt.Errorf("native function listing mapping or arguments mismatch")
					}
					info, err := os.Stat(filepath.Join(root, expected.Text))
					if err != nil {
						return err
					}
					listingBytes += info.Size()
					if !info.Mode().IsRegular() || info.Size() == 0 || listingBytes > int64(functionListingLimit(recorded)) {
						return fmt.Errorf("invalid function listing size")
					}
					listing, err := os.ReadFile(filepath.Join(root, expected.Text))
					if err != nil {
						return err
					}
					if !bytes.Equal(listing, []byte(expected.Listing)) {
						return fmt.Errorf("embedded function listing differs from tool output")
					}
					if _, err := os.Stat(filepath.Join(root, expected.ObjdumpLog)); err != nil {
						return err
					}
				}
			} else if len(d.Functions) != 0 {
				return fmt.Errorf("unexpected v1 function disassembly")
			}
			want.Disassembly = got.Disassembly
		} else if (recorded.Version == "native-image-disassembly-v1" || recorded.Version == "native-image-disassembly-v2" || recorded.Version == "native-image-disassembly-v3" && got.Image != nil && len(got.Image.Functions) > 0) && got.Status == "available" {
			return fmt.Errorf("missing disassembly for %s", got.Trial)
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("native export record differs from raw trial %d", i)
		}
		if got.Status == "available" {
			data, err := os.ReadFile(filepath.Join(root, got.Path))
			if err != nil {
				return err
			}
			if !bytes.Equal(data, trial.CodeImage.Data) {
				return fmt.Errorf("native image differs from raw trial %d", i)
			}
			if got.Disassembly != nil {
				_, machine, err := nativeELFTarget(got.Image.Architecture)
				if err != nil {
					return err
				}
				if err := verifyNativeObject(filepath.Join(root, got.Disassembly.Object), filepath.Join(root, got.Path), machine); err != nil {
					return err
				}
				if got.Disassembly.Text != "" {
					if _, err := os.Stat(filepath.Join(root, got.Disassembly.Text)); err != nil {
						return err
					}
				}
			}
		}
	}
	html, err := renderCodeHTML(recorded)
	if err != nil {
		return err
	}
	stored, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, html) {
		return fmt.Errorf("native code page differs from its dataset and renderer")
	}
	kind := "native-export"
	if recorded.Version != "native-image-export-v1" {
		kind = "native-disassembly"
	}
	return verifyNativeBuilder(root, kind, recorded.BuilderArchiveVersion)
}

func renderCodeHTML(r NativeExport) ([]byte, error) {
	t, err := template.New(nativeTemplate(r.Version)).Funcs(template.FuncMap{"rangeRatio": func(value *float64) string {
		if value == nil {
			return "unavailable"
		}
		return fmt.Sprintf("%.2f×", *value)
	}}).ParseFS(codeAssets, nativeTemplate(r.Version))
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	if err := t.Execute(&output, r); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func nativeTemplate(version string) string {
	if version == "native-image-disassembly-v3" {
		return "code_functions.html"
	}
	return "code.html"
}
func codeRendererHash() (string, error) { return codeRendererHashFor("") }
func codeRendererHashFor(version string) (string, error) {
	data, err := codeAssets.ReadFile(nativeTemplate(version))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(data)), nil
}
