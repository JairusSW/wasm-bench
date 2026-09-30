package publish

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/wasmbench/wasmbench/experiment"
	"github.com/wasmbench/wasmbench/protocol"
)

//go:embed code_compare.html
var codeCompareAssets embed.FS

const CodeComparisonVersion = "native-function-comparison-v1"
const codeComparisonInterpretation = "Exact retained compile snapshots, matched by workload contract, Wasm artifact digest, block and full function index. Backend/generation remain explicit on each side. Byte differences may include constants, relocations or address-dependent encodings; no normalization, semantic equivalence, instruction-only size, compiler counters or performance effect is inferred. Listings are sealed LLVM-produced diagnostics, not independently re-disassembled. Missing exports and unmatched functions remain visible. No timings are taken from code passes."

type ComparedNativeFunction struct {
	Function  protocol.CodeFunction    `json:"function"`
	SHA256    string                   `json:"sha256"`
	Listing   string                   `json:"listing,omitempty"`
	Expansion *NativeFunctionExpansion `json:"expansion,omitempty"`
}

type NativeFunctionDifference struct {
	WasmIndex             uint32                  `json:"wasm_index"`
	Status                string                  `json:"status"`
	Baseline              *ComparedNativeFunction `json:"baseline"`
	Candidate             *ComparedNativeFunction `json:"candidate"`
	SizeDelta             *int64                  `json:"size_delta_bytes"`
	BytesEqual            *bool                   `json:"bytes_equal"`
	FirstDifferenceOffset *uint64                 `json:"first_difference_offset"`
}

type NativeCodeDifference struct {
	Workload        string                     `json:"workload"`
	Block           int                        `json:"block"`
	ModuleSHA256    string                     `json:"module_sha256,omitempty"`
	Status          string                     `json:"status"`
	Reason          string                     `json:"reason,omitempty"`
	BaselineRecord  *int                       `json:"baseline_record"`
	CandidateRecord *int                       `json:"candidate_record"`
	BaselineStatus  string                     `json:"baseline_export_status"`
	CandidateStatus string                     `json:"candidate_export_status"`
	BaselineReason  string                     `json:"baseline_export_reason,omitempty"`
	CandidateReason string                     `json:"candidate_export_reason,omitempty"`
	Functions       []NativeFunctionDifference `json:"functions"`
}

type NativeCodeComparison struct {
	BuilderArchiveVersion  string                 `json:"builder_archive_version,omitempty"`
	Version                string                 `json:"version"`
	RendererSHA256         string                 `json:"renderer_sha256"`
	Interpretation         string                 `json:"interpretation"`
	BaselineHash           string                 `json:"baseline_checksums_sha256"`
	CandidateHash          string                 `json:"candidate_checksums_sha256"`
	BaselineRun            string                 `json:"baseline_run"`
	CandidateRun           string                 `json:"candidate_run"`
	BaselineConfiguration  experiment.Runtime     `json:"baseline_configuration"`
	CandidateConfiguration experiment.Runtime     `json:"candidate_configuration"`
	RunnerChanged          bool                   `json:"runner_changed"`
	Results                []NativeCodeDifference `json:"results"`
}

type nativeComparisonInput struct {
	report NativeExport
	bundle experiment.Bundle
	hash   string
}

func loadNativeComparisonInput(root string) (nativeComparisonInput, error) {
	var input nativeComparisonInput
	if err := VerifyNativeCode(root); err != nil {
		return input, err
	}
	if err := experiment.ReadJSON(filepath.Join(root, "native-code.json"), &input.report); err != nil {
		return input, err
	}
	b, err := experiment.Load(filepath.Join(root, "raw"))
	if err != nil {
		return input, err
	}
	input.bundle = b
	input.hash, err = experiment.DigestFile(filepath.Join(root, "checksums.json"))
	return input, err
}

func sameCodeWorkload(a, b protocol.Workload) bool {
	a.Artifact = ""
	b.Artifact = ""
	a.Source = ""
	b.Source = ""
	a.Provenance = nil
	b.Provenance = nil
	x, ex := json.Marshal(a)
	y, ey := json.Marshal(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}

func compareNativeInputs(a, b nativeComparisonInput, baseline, candidate string) (NativeCodeComparison, error) {
	x := NativeCodeComparison{Version: CodeComparisonVersion, Interpretation: codeComparisonInterpretation, BaselineHash: a.hash, CandidateHash: b.hash, BaselineRun: a.bundle.Manifest.ID, CandidateRun: b.bundle.Manifest.ID, Results: []NativeCodeDifference{}, RunnerChanged: a.bundle.Manifest.Lock.RunnerSHA256 != b.bundle.Manifest.Lock.RunnerSHA256}
	asset, err := codeCompareAssets.ReadFile("code_compare.html")
	if err != nil {
		return x, err
	}
	sum := sha256.Sum256(asset)
	x.RendererSHA256 = hex.EncodeToString(sum[:])
	ma, mb := a.bundle.Manifest, b.bundle.Manifest
	if ma.Kind != "measurement" || mb.Kind != "measurement" || ma.Lock.Options.Profile != "code" || mb.Lock.Options.Profile != "code" {
		return x, fmt.Errorf("native comparison requires code measurement evidence")
	}
	if !reflect.DeepEqual(ma.Host, mb.Host) || ma.Lock.Protocol != mb.Lock.Protocol {
		return x, fmt.Errorf("native comparison host, resource or protocol identity differs")
	}
	if err := experiment.MatchHostMeasurementPolicy(ma.Lock, mb.Lock); err != nil {
		return x, fmt.Errorf("native comparison host measurement policy differs: %w", err)
	}
	find := func(bundle experiment.Bundle, id string) (experiment.Runtime, error) {
		for _, r := range bundle.Manifest.Lock.Runtimes {
			if r.ID == id {
				return r, nil
			}
		}
		return experiment.Runtime{}, fmt.Errorf("native comparison runtime %q absent", id)
	}
	x.BaselineConfiguration, err = find(a.bundle, baseline)
	if err != nil {
		return x, err
	}
	x.CandidateConfiguration, err = find(b.bundle, candidate)
	if err != nil {
		return x, err
	}
	wa, wb := map[string]protocol.Workload{}, map[string]protocol.Workload{}
	for _, w := range ma.Lock.Workloads {
		wa[w.ID] = w
	}
	for _, w := range mb.Lock.Workloads {
		wb[w.ID] = w
	}
	type cell struct {
		workload string
		block    int
	}
	collect := func(input nativeComparisonInput, runtime string) (map[cell]int, error) {
		out := map[cell]int{}
		if len(input.report.Records) != len(input.bundle.Trials) {
			return nil, fmt.Errorf("native record/trial coverage differs")
		}
		for i, t := range input.bundle.Trials {
			if t.Runtime != runtime || t.Profile != "code" || t.Scenario != "compile" || t.Block < 0 {
				continue
			}
			key := cell{t.Workload, t.Block}
			if _, ok := out[key]; ok {
				return nil, fmt.Errorf("duplicate native compile trial cell")
			}
			out[key] = i
		}
		return out, nil
	}
	aa, err := collect(a, baseline)
	if err != nil {
		return x, err
	}
	bb, err := collect(b, candidate)
	if err != nil {
		return x, err
	}
	keys := map[cell]bool{}
	for key := range aa {
		keys[key] = true
	}
	for key := range bb {
		keys[key] = true
	}
	ordered := make([]cell, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].workload != ordered[j].workload {
			return ordered[i].workload < ordered[j].workload
		}
		return ordered[i].block < ordered[j].block
	})
	if len(ordered) == 0 {
		return x, fmt.Errorf("no selected native compile trials")
	}
	for _, key := range ordered {
		row := NativeCodeDifference{Workload: key.workload, Block: key.block, Status: "unavailable", BaselineStatus: "not_recorded", CandidateStatus: "not_recorded", Functions: []NativeFunctionDifference{}}
		ai, hasA := aa[key]
		bi, hasB := bb[key]
		if hasA {
			row.BaselineRecord = &ai
			row.BaselineStatus = a.report.Records[ai].Status
			row.BaselineReason = a.report.Records[ai].Reason
		}
		if hasB {
			row.CandidateRecord = &bi
			row.CandidateStatus = b.report.Records[bi].Status
			row.CandidateReason = b.report.Records[bi].Reason
		}
		w1, existsA := wa[key.workload]
		w2, existsB := wb[key.workload]
		if !existsA || !existsB || !sameCodeWorkload(w1, w2) {
			row.Status = "incomparable"
			row.Reason = "exact Wasm artifact or executable workload contract differs"
			x.Results = append(x.Results, row)
			continue
		}
		row.ModuleSHA256 = w1.SHA256
		functions := func(input nativeComparisonInput, index int, present bool) (map[uint32]*ComparedNativeFunction, error) {
			out := map[uint32]*ComparedNativeFunction{}
			if !present {
				return out, nil
			}
			record, trial := input.report.Records[index], input.bundle.Trials[index]
			if record.Status != "available" || trial.CodeImage == nil || (trial.CodeImage.Version != 2 && trial.CodeImage.Version != 3) {
				return out, nil
			}
			image := trial.CodeImage
			if err := image.Validate(row.ModuleSHA256); err != nil {
				return nil, err
			}
			for j, f := range image.Functions {
				sum := sha256.Sum256(image.Data[f.Offset : f.Offset+f.Length])
				entry := &ComparedNativeFunction{Function: f, SHA256: hex.EncodeToString(sum[:])}
				if record.Disassembly != nil && j < len(record.Disassembly.Functions) {
					entry.Listing = record.Disassembly.Functions[j].Listing
				}
				if record.Expansion != nil && j < len(record.Expansion.Functions) {
					e := record.Expansion.Functions[j]
					entry.Expansion = &e
				}
				out[f.WasmIndex] = entry
			}
			return out, nil
		}
		fa, err := functions(a, ai, hasA)
		if err != nil {
			return x, err
		}
		fb, err := functions(b, bi, hasB)
		if err != nil {
			return x, err
		}
		indices := map[uint32]bool{}
		for i := range fa {
			indices[i] = true
		}
		for i := range fb {
			indices[i] = true
		}
		sorted := make([]uint32, 0, len(indices))
		for i := range indices {
			sorted = append(sorted, i)
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
		matched := 0
		for _, i := range sorted {
			entry := NativeFunctionDifference{WasmIndex: i, Baseline: fa[i], Candidate: fb[i], Status: "unmatched"}
			if fa[i] != nil && fb[i] != nil {
				matched++
				delta := int64(fb[i].Function.Length) - int64(fa[i].Function.Length)
				leftRange, rightRange := fa[i].Function, fb[i].Function
				left := a.bundle.Trials[ai].CodeImage.Data[leftRange.Offset : leftRange.Offset+leftRange.Length]
				right := b.bundle.Trials[bi].CodeImage.Data[rightRange.Offset : rightRange.Offset+rightRange.Length]
				equal := bytes.Equal(left, right)
				if !equal {
					offset := uint64(0)
					for offset < uint64(len(left)) && offset < uint64(len(right)) && left[offset] == right[offset] {
						offset++
					}
					entry.FirstDifferenceOffset = &offset
				}
				entry.SizeDelta = &delta
				entry.BytesEqual = &equal
				entry.Status = "changed"
				if equal {
					entry.Status = "identical_bytes"
				}
			}
			row.Functions = append(row.Functions, entry)
		}
		if hasA && hasB && a.report.Records[ai].Image != nil && b.report.Records[bi].Image != nil && a.report.Records[ai].Image.Architecture != b.report.Records[bi].Image.Architecture {
			return x, fmt.Errorf("native code architectures differ")
		}
		attributedEmpty := hasA && hasB && a.report.Records[ai].Status == "available" && b.report.Records[bi].Status == "available" && a.report.Records[ai].Image != nil && b.report.Records[bi].Image != nil && (a.report.Records[ai].Image.Version == 2 || a.report.Records[ai].Image.Version == 3) && (b.report.Records[bi].Image.Version == 2 || b.report.Records[bi].Image.Version == 3)
		if matched == len(sorted) && (len(sorted) > 0 || attributedEmpty) {
			row.Status = "matched"
		} else {
			row.Reason = "missing trial, unsupported function attribution, or unmatched exported function coverage"
			if matched > 0 {
				row.Status = "partial"
			}
		}
		x.Results = append(x.Results, row)
	}
	return x, nil
}

func renderNativeComparison(x NativeCodeComparison) ([]byte, error) {
	t, err := template.New("code_compare.html").Funcs(template.FuncMap{"shellArg": func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }, "signedDelta": func(n *int64) string {
		if n == nil {
			return "unavailable"
		}
		return fmt.Sprintf("%+d", *n)
	}}).ParseFS(codeCompareAssets, "code_compare.html")
	if err != nil {
		return nil, err
	}
	baseline, err := json.MarshalIndent(x.BaselineConfiguration, "", "  ")
	if err != nil {
		return nil, err
	}
	candidate, err := json.MarshalIndent(x.CandidateConfiguration, "", "  ")
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	err = t.Execute(&output, struct {
		NativeCodeComparison
		BaselineJSON, CandidateJSON string
	}{x, string(baseline), string(candidate)})
	return output.Bytes(), err
}

// CompareNativeCode packages verified exports and recomputed byte/range evidence.
// It never executes code or treats a code-profile timer as latency evidence.
func CompareNativeCode(baselineReport, candidateReport, baselineRuntime, candidateRuntime, out string) error {
	if err := reportOutsideInputs([]string{baselineReport, candidateReport}, out); err != nil {
		return err
	}
	a, err := loadNativeComparisonInput(baselineReport)
	if err != nil {
		return err
	}
	b, err := loadNativeComparisonInput(candidateReport)
	if err != nil {
		return err
	}
	if _, err = compareNativeInputs(a, b, baselineRuntime, candidateRuntime); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	if err = os.Mkdir(out, 0755); err != nil {
		return err
	}
	for _, side := range []struct{ src, name, hash string }{{baselineReport, "baseline", a.hash}, {candidateReport, "candidate", b.hash}} {
		dst := filepath.Join(out, "evidence", side.name)
		if err = os.CopyFS(dst, os.DirFS(side.src)); err != nil {
			return err
		}
		copied, err := loadNativeComparisonInput(dst)
		if err != nil {
			return err
		}
		if copied.hash != side.hash {
			return fmt.Errorf("native comparison evidence changed while copying")
		}
		if side.name == "baseline" {
			a = copied
		} else {
			b = copied
		}
	}
	x, err := compareNativeInputs(a, b, baselineRuntime, candidateRuntime)
	if err != nil {
		return err
	}
	x.BuilderArchiveVersion = FamilyBuilderVersion
	if err = experiment.WriteJSON(filepath.Join(out, "data.json"), x); err != nil {
		return err
	}
	page, err := renderNativeComparison(x)
	if err != nil {
		return err
	}
	if err = writeDiagnostic(filepath.Join(out, "index.html"), page); err != nil {
		return err
	}
	if err = sealFamilyReport(out, "native-comparison"); err != nil {
		return err
	}
	return VerifyNativeComparison(out)
}

func VerifyNativeComparison(root string) error {
	if err := experiment.Verify(root); err != nil {
		return err
	}
	var saved NativeCodeComparison
	if err := experiment.ReadJSON(filepath.Join(root, "data.json"), &saved); err != nil {
		return err
	}
	a, err := loadNativeComparisonInput(filepath.Join(root, "evidence/baseline"))
	if err != nil {
		return err
	}
	b, err := loadNativeComparisonInput(filepath.Join(root, "evidence/candidate"))
	if err != nil {
		return err
	}
	want, err := compareNativeInputs(a, b, saved.BaselineConfiguration.ID, saved.CandidateConfiguration.ID)
	if err != nil {
		return err
	}
	want.BuilderArchiveVersion = saved.BuilderArchiveVersion
	if !reflect.DeepEqual(saved, want) {
		return fmt.Errorf("native comparison differs from sealed input evidence")
	}
	page, err := renderNativeComparison(want)
	if err != nil {
		return err
	}
	stored, err := os.ReadFile(filepath.Join(root, "index.html"))
	if err != nil {
		return err
	}
	if !bytes.Equal(page, stored) {
		return fmt.Errorf("native comparison page differs from derived dataset")
	}
	return verifyNativeBuilder(root, "native-comparison", saved.BuilderArchiveVersion)
}
