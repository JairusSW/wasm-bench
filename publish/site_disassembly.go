package publish

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf8"
)

// The existing sealed native exporter owns LLVM execution and range mapping.
// Site export only transports its verified diagnostics, never invokes tools.
type siteDisassemblyScope struct {
	Location         string `json:"location"`
	CodePassID       string `json:"codePassId"`
	CodeSealSHA256   string `json:"codeSealSha256"`
	NativeSealSHA256 string `json:"nativeSealSha256"`
	Verification     string `json:"verification"`
}

func siteDisassemblies(report, external string, code *CodeSource) (map[string]NativeExportRecord, []NativeTool, string, *siteDisassemblyScope, error) {
	records := map[string]NativeExportRecord{}
	if report == "" && external == "" {
		return records, nil, "", nil, nil
	}
	root := filepath.Join(report, "code")
	location := "embedded-report"
	if external != "" {
		root = external
		location = "external-archive"
	}
	b, err := os.ReadFile(filepath.Join(root, "native-code.json"))
	if os.IsNotExist(err) && external == "" {
		return records, nil, "", nil, nil
	}
	if err != nil {
		return nil, nil, "", nil, err
	}
	var source NativeExport
	if err = json.Unmarshal(b, &source); err != nil {
		return nil, nil, "", nil, err
	}
	if source.Version != "native-image-disassembly-v2" && source.Version != "native-image-disassembly-v3" {
		if external != "" {
			return nil, nil, "", nil, fmt.Errorf("external native archive lacks supported per-function disassembly")
		}
		return records, nil, "", nil, nil
	}
	if code == nil || code.ID == "" || source.Run != code.ID {
		return nil, nil, "", nil, fmt.Errorf("native archive code-pass identity differs")
	}
	if err = VerifyNativeCode(root); err != nil {
		return nil, nil, "", nil, err
	}
	expected, err := os.ReadFile(filepath.Join(report, "code", "raw", "checksums.json"))
	if err != nil {
		return nil, nil, "", nil, err
	}
	if source.SourceChecksumsSHA256 != siteHash(expected) {
		return nil, nil, "", nil, fmt.Errorf("native archive source seal differs from measurement code pass")
	}
	nativeSeal, err := os.ReadFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		return nil, nil, "", nil, err
	}
	scope := &siteDisassemblyScope{Location: location, CodePassID: source.Run, CodeSealSHA256: source.SourceChecksumsSHA256, NativeSealSHA256: siteHash(nativeSeal), Verification: "producer-asserted"}
	for _, record := range source.Records {
		if _, ok := records[record.Trial]; ok {
			return nil, nil, "", nil, fmt.Errorf("duplicate disassembly trial")
		}
		records[record.Trial] = record
	}
	return records, source.Tools, source.Version, scope, nil
}

type siteNativeFunction struct {
	protocol.CodeFunction
	Disassembly string `json:"disassembly,omitempty"`
}

func siteNativeFunctions(image protocol.CodeImage, source *NativeExportRecord, tools []NativeTool, sourceVersion string, object func(string, any) (string, error), scopes ...*siteDisassemblyScope) ([]string, []int, error) {
	rows := []siteNativeFunction{}
	derivative := false
	if source != nil {
		if source.Image == nil || source.Image.SHA256 != image.SHA256 || source.Image.ModuleSHA256 != image.ModuleSHA256 || source.Image.Architecture != image.Architecture || source.Image.Backend != image.Backend || !reflect.DeepEqual(source.Image.Functions, image.Functions) || source.Disassembly == nil || len(source.Disassembly.Functions) != len(image.Functions) || len(tools) != 2 || (sourceVersion != "native-image-disassembly-v2" && sourceVersion != "native-image-disassembly-v3") {
			return nil, nil, fmt.Errorf("offline derivative source differs")
		}
		for _, tool := range tools {
			digest, err := hex.DecodeString(tool.SHA256)
			if err != nil || len(digest) != 32 || tool.Version == "" || len(tool.Version) > 4096 || tool.SHA256 != strings.ToLower(tool.SHA256) {
				return nil, nil, fmt.Errorf("offline tool identity exceeds contract")
			}
		}
		derivative = true
	}
	for i, function := range image.Functions {
		row := siteNativeFunction{CodeFunction: function}
		if derivative {
			entry := source.Disassembly.Functions[i]
			if !reflect.DeepEqual(entry.Function, function) || entry.Listing == "" || len(entry.Listing) > 64<<20 || !utf8.ValidString(entry.Listing) {
				return nil, nil, fmt.Errorf("offline derivative function differs")
			}
			expected := []string{"--disassemble", "--disassemble-zeroes", "--section=.text", fmt.Sprintf("--start-address=%d", function.Offset), fmt.Sprintf("--stop-address=%d", function.Offset+function.Length)}
			if len(entry.ObjdumpArgs) != 6 {
				return nil, nil, fmt.Errorf("offline function arguments differ")
			}
			for i, arg := range entry.ObjdumpArgs {
				if arg == "" || len(arg) > 4096 || i < 5 && arg != expected[i] {
					return nil, nil, fmt.Errorf("offline function arguments differ")
				}
			}
			lines := strings.SplitAfter(entry.Listing, "\n")
			if lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}
			refs := []string{}
			chunks := []map[string]any{}
			for start := 0; start < len(lines); {
				end := start
				size := 0
				for end < len(lines) && end-start < 256 {
					line := lines[end]
					if len(line) > 16<<10 {
						return nil, nil, fmt.Errorf("disassembly line exceeds ceiling")
					}
					encoded, err := siteJSON(line)
					if err != nil {
						return nil, nil, err
					}
					if size+len(encoded) > (128<<10)-1024 {
						break
					}
					size += len(encoded)
					end++
				}
				id, err := object("evidence", map[string]any{"kind": "native-disassembly-lines", "lines": lines[start:end]})
				if err != nil {
					return nil, nil, err
				}
				refs = append(refs, id)
				chunks = append(chunks, map[string]any{"sha256": id, "lines": end - start})
				if len(refs) > 4096 {
					return nil, nil, fmt.Errorf("disassembly exceeds 4096 line chunks")
				}
				start = end
			}
			toolIDs := []map[string]string{}
			for _, tool := range tools {
				toolIDs = append(toolIDs, map[string]string{"sha256": tool.SHA256, "version": tool.Version})
			}
			payload := map[string]any{"kind": "native-function-disassembly", "version": "llvm-function-listing-v1", "sourceVersion": sourceVersion, "imageSha256": image.SHA256, "moduleSha256": image.ModuleSHA256, "architecture": image.Architecture, "function": function, "tools": toolIDs, "arguments": entry.ObjdumpArgs, "interpretation": siteDisassemblyInterpretation(sourceVersion), "textSha256": siteHash([]byte(entry.Listing)), "bytes": len(entry.Listing), "lines": len(lines), "chunks": chunks, "references": refs}
			if len(scopes) > 0 && scopes[0] != nil {
				payload["source"] = scopes[0]
			}
			id, err := object("evidence", payload)
			if err != nil {
				return nil, nil, err
			}
			row.Disassembly = id
		}
		rows = append(rows, row)
	}
	refs := []string{}
	counts := []int{}
	for start := 0; start < len(rows); {
		end, size := start, 0
		for end < len(rows) && end-start < 128 {
			encoded, err := siteJSON(rows[end])
			if err != nil {
				return nil, nil, err
			}
			if len(encoded) > SiteChunkBytes-16384 {
				return nil, nil, fmt.Errorf("native function descriptor exceeds ceiling")
			}
			if size+len(encoded) > SiteChunkBytes-16384 {
				break
			}
			size += len(encoded)
			end++
		}
		var payload any = rows[start:end]
		if derivative {
			links := []string{}
			for _, row := range rows[start:end] {
				if row.Disassembly != "" {
					links = append(links, row.Disassembly)
				}
			}
			payload = map[string]any{"kind": "native-functions-v2", "functions": rows[start:end], "references": links}
		}
		id, err := object("evidence", payload)
		if err != nil {
			return nil, nil, err
		}
		refs = append(refs, id)
		counts = append(counts, end-start)
		start = end
	}
	return refs, counts, nil
}

func siteDisassemblyInterpretation(version string) string {
	if version == "native-image-disassembly-v3" {
		return nativeFunctionDisassemblyInterpretation
	}
	return nativeDisassemblyInterpretation
}
