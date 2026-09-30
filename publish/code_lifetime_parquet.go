package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/parquet-go/parquet-go"
	"github.com/wasmbench/wasmbench/experiment"
	"io"
	"os"
	"path/filepath"
)

const CodeLifetimeExportVersion = "native-code-publication-parquet-v1"

// A trial outcome retains missing evidence; optional values distinguish it
// from an actual completed zero-capacity retirement/checkpoint.
type CodeLifetimeRow struct {
	Version          string  `parquet:"export_version"`
	RowKind          string  `parquet:"row_kind"`
	Run              string  `parquet:"run"`
	Trial            string  `parquet:"trial"`
	Runtime          string  `parquet:"runtime_configuration"`
	Workload         string  `parquet:"workload"`
	Status           string  `parquet:"trial_status"`
	Reason           string  `parquet:"trial_reason"`
	Module           string  `parquet:"module_sha256"`
	Image            string  `parquet:"image_sha256"`
	Collector        string  `parquet:"collector"`
	CollectorVersion string  `parquet:"collector_version"`
	Scope            string  `parquet:"scope"`
	Quality          string  `parquet:"quality"`
	Kind             *string `parquet:"event_or_checkpoint_kind,optional"`
	Sequence         *uint64 `parquet:"sequence,optional"`
	ElapsedNS        *int64  `parquet:"elapsed_ns,optional"`
	Publication      *uint64 `parquet:"publication,optional"`
	Address          *string `parquet:"address_decimal,optional"`
	Capacity         *uint64 `parquet:"capacity_bytes,optional"`
	Active           *uint64 `parquet:"active_published_capacity_bytes,optional"`
	Cumulative       *uint64 `parquet:"cumulative_published_capacity_bytes,optional"`
	EventCount       *uint64 `parquet:"observed_event_count,optional"`
	EvidenceJSON     *string `parquet:"lifetime_evidence_json,optional"`
}

func codeLifetimeRows(b experiment.Bundle) ([]CodeLifetimeRow, error) {
	var rows []CodeLifetimeRow
	for _, t := range b.Trials {
		if t.CodeLifetime == nil && t.Scenario != "code-lifetime" {
			continue
		}
		r := CodeLifetimeRow{Version: CodeLifetimeExportVersion, RowKind: "trial_outcome", Run: b.Manifest.ID, Trial: t.ID, Runtime: t.Runtime, Workload: t.Workload, Status: t.Status, Reason: t.Reason}
		l := t.CodeLifetime
		if l == nil {
			rows = append(rows, r)
			continue
		}
		if t.Profile != "code" || t.Status != "ok" || t.Block < 0 || t.Scenario != "code-lifetime" || t.CodeImage == nil {
			return nil, fmt.Errorf("code lifetime outside successful diagnostic trial")
		}
		if err := l.Validate(t.CodeImage.ModuleSHA256, t.CodeImage); err != nil {
			return nil, err
		}
		r.Module = l.ModuleSHA256
		r.Image = l.ImageSHA256
		r.Collector = l.Collector
		r.CollectorVersion = l.CollectorVersion
		r.Scope = l.Scope
		r.Quality = l.Quality
		raw, err := json.Marshal(l)
		if err != nil {
			return nil, err
		}
		evidence := string(raw)
		r.EvidenceJSON = &evidence
		rows = append(rows, r)
		r.EvidenceJSON = nil
		for _, e := range l.Events {
			row := r
			row.RowKind = "publication_event"
			row.Kind = &e.Kind
			row.Sequence = &e.Sequence
			row.ElapsedNS = &e.ElapsedNS
			row.Publication = &e.Publication
			row.Address = &e.Address
			row.Capacity = &e.Capacity
			row.Active = &e.ActiveCapacity
			row.Cumulative = &e.CumulativeCapacity
			rows = append(rows, row)
		}
		for i, c := range l.Checkpoints {
			row := r
			row.RowKind = "ownership_checkpoint"
			index := uint64(i)
			row.Sequence = &index
			row.Kind = &c.Stage
			row.ElapsedNS = &c.ElapsedNS
			row.EventCount = &c.EventCount
			row.Active = &c.ActiveCapacity
			row.Cumulative = &c.CumulativeCapacity
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func ExportCodeLifetimes(b experiment.Bundle, path string) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return writeCodeLifetimeParquet(b, f)
}

func writeCodeLifetimeParquet(b experiment.Bundle, out io.Writer) (err error) {
	rows, err := codeLifetimeRows(b)
	if err != nil {
		return err
	}
	w := parquet.NewGenericWriter[CodeLifetimeRow](out, reportParquetWriterOption())
	defer func() { err = errors.Join(err, w.Close()) }()
	_, err = w.Write(rows)
	return err
}

func verifyCodeLifetimeParquet(b experiment.Bundle, root string) error {
	sum := sha256.New()
	if err := writeCodeLifetimeParquet(b, sum); err != nil {
		return err
	}
	actual, err := experiment.DigestFile(filepath.Join(root, "code-lifetimes.parquet"))
	if err != nil {
		return err
	}
	if actual != hex.EncodeToString(sum.Sum(nil)) {
		return fmt.Errorf("code lifetime Parquet differs from raw evidence and export version")
	}
	return nil
}
