package corpus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/protocol"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ImportWago retains every selected entry, including unsupported host contracts.
// Artifacts stay in the source checkout until a run copies their verified bytes.
func ImportWago(root string, ids []string) ([]protocol.Workload, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	base := filepath.Join(root, "corpus")
	b, e := os.ReadFile(filepath.Join(base, "catalog.json"))
	if e != nil {
		return nil, e
	}
	var catalog struct {
		Schema     int               `json:"schema"`
		Benchmarks []json.RawMessage `json:"benchmarks"`
		Checks     []json.RawMessage `json:"checks"`
	}
	if e = json.Unmarshal(b, &catalog); e != nil {
		return nil, e
	}
	if catalog.Schema != 1 {
		return nil, fmt.Errorf("unknown Wago catalog schema")
	}
	checks := map[string]json.RawMessage{}
	for _, raw := range catalog.Checks {
		var c struct {
			ID string `json:"id"`
		}
		if e = json.Unmarshal(raw, &c); e != nil {
			return nil, e
		}
		checks[c.ID] = raw
	}
	selected := map[string]bool{}
	for _, id := range ids {
		selected[id] = false
	}
	revision := "unknown"
	cmd := exec.Command("git", "-C", root, "rev-parse", "HEAD")
	if v, err := cmd.Output(); err == nil {
		revision = string(v)
	}
	provenance, _ := json.Marshal(map[string]string{"catalog_sha256": Hash(b), "source_checkout": root, "source_revision": revision, "importer": "wago-catalog-v1"})
	var out []protocol.Workload
	for _, raw := range catalog.Benchmarks {
		var entry struct {
			ID       string `json:"id"`
			Artifact string `json:"artifact"`
			SHA      string `json:"artifact_sha256"`
			Init     string `json:"init"`
			Exec     []struct {
				Export string          `json:"export"`
				Args   protocol.Values `json:"args"`
				Want   protocol.Values `json:"want"`
			} `json:"exec"`
			Semantic []string        `json:"semantic_exec"`
			Command  json.RawMessage `json:"command"`
			Source   struct {
				License string `json:"license"`
			} `json:"source"`
		}
		if e = json.Unmarshal(raw, &entry); e != nil {
			return nil, e
		}
		if len(selected) > 0 {
			if _, ok := selected[entry.ID]; !ok {
				continue
			}
			selected[entry.ID] = true
		}
		if !filepath.IsLocal(entry.Artifact) {
			return nil, fmt.Errorf("unsafe artifact path")
		}
		path := filepath.Join(base, entry.Artifact)
		data, e := os.ReadFile(path)
		if e != nil {
			return nil, e
		}
		if Hash(data) != entry.SHA {
			return nil, fmt.Errorf("Wago artifact digest mismatch: %s", entry.ID)
		}
		w := protocol.Workload{Schema: 1, ID: "wago/" + entry.ID, Family: "algorithms", Artifact: path, SHA256: entry.SHA, ABI: "core", Features: []string{}, Initialize: entry.Init, WorkUnit: "invocation", Units: 1, Reset: "fresh_instance_per_sample", License: entry.Source.License, Source: entry.Artifact, Generator: "wago-catalog-v1", OriginalContract: raw, Provenance: provenance}
		// This named upstream family has AssemblyScript's abort host contract.
		// Do not infer a host ABI from an initialization export alone.
		if strings.HasPrefix(entry.Artifact, "workloads/assemblyscript/") {
			w.HostProfile = protocol.AssemblyScriptAbortProfile
		}
		if w.License == "" {
			w.License = "see upstream artifact provenance; not inferred from repository license"
		}
		if len(entry.Exec) > 0 {
			for _, call := range entry.Exec {
				x := w
				x.ID += "/" + call.Export
				x.Export = call.Export
				x.Args = call.Args
				x.Oracle = protocol.Oracle{Kind: "exact_u64", Expected: call.Want}
				out = append(out, x)
			}
			continue
		}
		if len(entry.Semantic) > 0 {
			for _, id := range entry.Semantic {
				raw, ok := checks[id]
				if !ok {
					return nil, fmt.Errorf("missing semantic oracle %s", id)
				}
				var c struct {
					Invoke struct {
						OutputPointerExport string          `json:"output_ptr_export"`
						InputPointerExport  string          `json:"input_ptr_export"`
						Input               *string         `json:"input"`
						Export              string          `json:"export"`
						Args                protocol.Values `json:"args"`
					} `json:"invoke"`
					Expect struct {
						Return []string               `json:"return"`
						Memory []protocol.MemoryCheck `json:"memory"`
					} `json:"expect"`
				}
				if e = json.Unmarshal(raw, &c); e != nil {
					return nil, e
				}
				x := w
				x.ID = "wago/" + id
				x.Export = c.Invoke.Export
				x.Args = c.Invoke.Args
				if c.Invoke.Input != nil {
					x.Input = &protocol.MemoryInput{PointerExport: c.Invoke.InputPointerExport, Hex: *c.Invoke.Input}
				}
				x.OriginalContract = raw
				x.Oracle = protocol.Oracle{Kind: "exact_u64", Memory: c.Expect.Memory, OutputPointerExport: c.Invoke.OutputPointerExport}
				// Keep richer contracts visible until their input/relative-pointer/vector
				// semantics are implemented. Never silently drop part of an oracle.
				var full map[string]json.RawMessage
				json.Unmarshal(raw, &full)
				var invoke map[string]json.RawMessage
				json.Unmarshal(full["invoke"], &invoke)
				var expect map[string]json.RawMessage
				json.Unmarshal(full["expect"], &expect)
				for key := range invoke {
					if key != "export" && key != "args" && key != "output_ptr_export" && key != "input" && key != "input_ptr_export" {
						x.UnsupportedReason = "semantic contract requires " + key
					}
				}
				for key := range expect {
					if key != "return" && key != "memory" {
						x.UnsupportedReason = "semantic oracle requires " + key
					}
				}
				var memories []map[string]json.RawMessage
				json.Unmarshal(expect["memory"], &memories)
				for _, memory := range memories {
					for key := range memory {
						if key != "offset" && key != "hex" {
							x.UnsupportedReason = "memory oracle requires " + key
						}
					}
				}
				if len(expect) == 0 {
					x.UnsupportedReason = "semantic contract has no supported exact oracle"
				}
				if vectorRaw, ok := invoke["vectors"]; ok {
					var vectors protocol.VectorContract
					decoder := json.NewDecoder(bytes.NewReader(vectorRaw))
					decoder.DisallowUnknownFields()
					err := decoder.Decode(&vectors)
					if err == nil {
						_, err = protocol.PrepareVectors(vectors, 64<<20)
					}
					if err == nil && len(invoke) == 2 && len(expect) == 0 {
						x.Vectors = &vectors
						x.VectorByteBudget = 64 << 20
						x.WorkUnit = "vector_sequence"
						x.Oracle = protocol.Oracle{Kind: "exact_vectors"}
						x.UnsupportedReason = ""
					} else {
						x.UnsupportedReason = "unsupported or ambiguous vector contract"
					}
				}
				if x.UnsupportedReason != "" {
					x.Oracle.Kind = "unsupported"
				}
				for _, s := range c.Expect.Return {
					n, e := strconv.ParseUint(s, 0, 64)
					if e != nil {
						return nil, e
					}
					x.Oracle.Expected = append(x.Oracle.Expected, n)
				}
				out = append(out, x)
			}
			continue
		}
		w.Oracle.Kind = "unsupported"
		w.UnsupportedReason = "compile-only entry has no executable oracle"
		if len(entry.Command) > 0 {
			w.ABI = "wasi-command"
			w.Family = "applications"
			w.UnsupportedReason = "command host, fixtures and output oracle require command ABI support"
			var command struct {
				Runtime string `json:"runtime"`
			}
			json.Unmarshal(entry.Command, &command)
			if command.Runtime == "emscripten" {
				w.ABI = "emscripten"
			}
			if err := importWagoCommand(base, entry.ID, entry.Command, &w); err != nil {
				return nil, fmt.Errorf("%s: %w", entry.ID, err)
			}
		}
		out = append(out, w)
	}
	for id, found := range selected {
		if !found {
			return nil, fmt.Errorf("unknown Wago workload %q", id)
		}
	}
	return out, nil
}
