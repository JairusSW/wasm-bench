package publish

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const SiteFileChunkBytes = 1024 * 1024
const SiteFileBytes = 1024 * 1024 * 1024

var siteAnalyticalFiles = []string{"samples.parquet", "throughput.parquet", "observations.parquet", "counters.parquet", "engine-events.parquet", "code-lifetimes.parquet", "memory-samples.parquet", "memory-observations.parquet"}

type SiteFileChunk struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}
type SiteReportFile struct {
	Schema    int             `json:"schema"`
	Kind      string          `json:"kind"`
	ReportID  string          `json:"reportId"`
	Name      string          `json:"name"`
	MediaType string          `json:"mediaType"`
	Encoding  string          `json:"encoding"`
	SHA256    string          `json:"sha256"`
	Bytes     int64           `json:"bytes"`
	Chunks    []SiteFileChunk `json:"chunks"`
}

// Only existing sealed analytical files are projected. No analytical writer or
// archived executable runs here, and memory exports are never inferred.
func siteReportFiles(report string, seal []byte, reportID string, binary func([]byte) (string, error), object func(string, any) (string, error)) (map[string]string, error) {
	files := map[string]string{}
	if report == "" {
		return files, nil
	}
	var hashes map[string]string
	if err := json.Unmarshal(seal, &hashes); err != nil {
		return nil, err
	}
	for _, name := range siteAnalyticalFiles {
		want, exists := hashes[name]
		if !exists {
			continue
		}
		if len(want) != 64 {
			return nil, fmt.Errorf("invalid analytical file seal")
		}
		path := filepath.Join(report, name)
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > SiteFileBytes {
			return nil, fmt.Errorf("analytical file exceeds bounded regular-file contract")
		}
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		opened, err := f.Stat()
		if err != nil || !os.SameFile(info, opened) {
			f.Close()
			return nil, fmt.Errorf("analytical file changed during export")
		}
		descriptor := SiteReportFile{Schema: 1, Kind: "report-file", ReportID: reportID, Name: name, MediaType: "application/vnd.apache.parquet", Encoding: "identity", SHA256: want, Bytes: info.Size(), Chunks: []SiteFileChunk{}}
		hash := sha256.New()
		var copied int64
		buf := make([]byte, SiteFileChunkBytes)
		for {
			n, readErr := io.ReadFull(f, buf)
			if n > 0 {
				copied += int64(n)
				if copied > SiteFileBytes {
					f.Close()
					return nil, fmt.Errorf("analytical file grew beyond ceiling")
				}
				hash.Write(buf[:n])
				digest, e := binary(buf[:n])
				if e != nil {
					f.Close()
					return nil, e
				}
				descriptor.Chunks = append(descriptor.Chunks, SiteFileChunk{digest, n})
			}
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				break
			}
			if readErr != nil {
				f.Close()
				return nil, readErr
			}
		}
		if err = f.Close(); err != nil {
			return nil, err
		}
		if copied != descriptor.Bytes || hex.EncodeToString(hash.Sum(nil)) != want {
			return nil, fmt.Errorf("analytical file differs from verified seal")
		}
		root, err := siteID(descriptor)
		if err != nil {
			return nil, err
		}
		data, err := siteJSON(descriptor)
		if err != nil {
			return nil, err
		}
		_, err = object("record", SiteRecord{Kind: "report-file", ID: root, Data: data})
		if err != nil {
			return nil, err
		}
		files[name] = root
	}
	return files, nil
}
