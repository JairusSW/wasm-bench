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
func siteDisassemblies(report string) (map[string]NativeExportRecord, []NativeTool, string, error) {
	records := map[string]NativeExportRecord{}
	if report == "" {
		return records, nil, "", nil
	}
	root := filepath.Join(report, "code")
	b, err := os.ReadFile(filepath.Join(root, "native-code.json"))
	if os.IsNotExist(err) {
		return records, nil, "", nil
	}
	if err != nil {
		return nil, nil, "", err
	}
	var source NativeExport
	if err = json.Unmarshal(b, &source); err != nil {
		return nil, nil, "", err
	}
	if source.Version != "native-image-disassembly-v2" {
		return records, nil, "", nil
	}
	if err = VerifyNativeCode(root); err != nil {
		return nil, nil, "", err
	}
	for _, record := range source.Records {
		if _, exists := records[record.Trial]; exists {
			return nil, nil, "", fmt.Errorf("duplicate disassembly trial")
		}
		records[record.Trial] = record
	}
	return records, source.Tools, source.Version, nil
}

type siteNativeFunction struct {
	protocol.CodeFunction
	Disassembly string `json:"disassembly,omitempty"`
}

func siteNativeFunctions(image protocol.CodeImage, source *NativeExportRecord, tools []NativeTool, sourceVersion string, object func(string, any) (string, error)) ([]string, []int, error) {
	rows := []siteNativeFunction{}
	derivative := false
	if source != nil {
		if source.Image == nil || source.Image.SHA256 != image.SHA256 || source.Image.ModuleSHA256 != image.ModuleSHA256 || source.Image.Architecture != image.Architecture || source.Image.Backend != image.Backend || !reflect.DeepEqual(source.Image.Functions, image.Functions) || source.Disassembly == nil || len(source.Disassembly.Functions) != len(image.Functions) || len(tools) != 2 || sourceVersion != "native-image-disassembly-v2" {
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
			id, err := object("evidence", map[string]any{"kind": "native-function-disassembly", "version": "llvm-function-listing-v1", "sourceVersion": sourceVersion, "imageSha256": image.SHA256, "moduleSha256": image.ModuleSHA256, "architecture": image.Architecture, "function": function, "tools": toolIDs, "arguments": entry.ObjdumpArgs, "interpretation": nativeDisassemblyInterpretation, "textSha256": siteHash([]byte(entry.Listing)), "bytes": len(entry.Listing), "lines": len(lines), "chunks": chunks, "references": refs})
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
