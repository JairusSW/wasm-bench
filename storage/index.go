// Package storage provides a rebuildable SQLite index and durable local job queue.
// Immutable bundles remain authoritative; the database is never the raw evidence.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/wasmbench/wasmbench/experiment"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"time"
)

type Index struct{ db *sql.DB }

func Open(path string) (*Index, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return nil, e
	}
	db, e := sql.Open("sqlite", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	_, e = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;
CREATE TABLE IF NOT EXISTS runs(id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, created TEXT NOT NULL, kind TEXT NOT NULL, profile TEXT NOT NULL, lock_sha256 TEXT NOT NULL, trial_count INTEGER NOT NULL, successful INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS jobs(id INTEGER PRIMARY KEY AUTOINCREMENT, state TEXT NOT NULL DEFAULT 'pending', lock_json BLOB NOT NULL, artifact_base TEXT NOT NULL, output_path TEXT NOT NULL UNIQUE, created TEXT NOT NULL, started TEXT, finished TEXT, error TEXT NOT NULL DEFAULT '');`)
	if e != nil {
		db.Close()
		return nil, e
	}
	return &Index{db: db}, nil
}
func (i *Index) Close() error { return i.db.Close() }
func (i *Index) AddRun(path string) error {
	path, e := filepath.Abs(path)
	if e != nil {
		return e
	}
	b, e := experiment.Load(path)
	if e != nil {
		return e
	}
	success, count := 0, 0
	for _, t := range b.Trials {
		if t.Block >= 0 {
			count++
			if t.Status == "ok" {
				success++
			}
		}
	}
	result, e := i.db.Exec(`INSERT INTO runs(id,path,created,kind,profile,lock_sha256,trial_count,successful) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET path=excluded.path WHERE runs.lock_sha256=excluded.lock_sha256 AND runs.created=excluded.created`, b.Manifest.ID, path, b.Manifest.Created.Format(time.RFC3339Nano), b.Manifest.Kind, b.Manifest.Lock.Options.Profile, b.Manifest.LockSHA256, count, success)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n == 0 {
		return fmt.Errorf("run ID %q already indexes different evidence; use a unique output directory name", b.Manifest.ID)
	}
	return e
}

type Run struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Created    string `json:"created"`
	Kind       string `json:"kind"`
	Profile    string `json:"profile"`
	LockSHA256 string `json:"lock_sha256"`
	Trials     int    `json:"trials"`
	Successful int    `json:"successful"`
}

func (i *Index) Runs() ([]Run, error) {
	rows, e := i.db.Query(`SELECT id,path,created,kind,profile,lock_sha256,trial_count,successful FROM runs ORDER BY created DESC`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Run{}
	for rows.Next() {
		var r Run
		if e = rows.Scan(&r.ID, &r.Path, &r.Created, &r.Kind, &r.Profile, &r.LockSHA256, &r.Trials, &r.Successful); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (i *Index) Enqueue(l experiment.Lock, base, out string) (int64, error) {
	if e := experiment.ValidateLock(l); e != nil {
		return 0, e
	}
	if e := experiment.VerifyInputs(l, base); e != nil {
		return 0, e
	}
	base, e := filepath.Abs(base)
	if e != nil {
		return 0, e
	}
	out, e = filepath.Abs(out)
	if e != nil {
		return 0, e
	}
	b, e := json.Marshal(l)
	if e != nil {
		return 0, e
	}
	r, e := i.db.Exec(`INSERT INTO jobs(lock_json,artifact_base,output_path,created) VALUES(?,?,?,?)`, b, base, out, time.Now().UTC().Format(time.RFC3339Nano))
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}

type Job struct {
	ID      int64  `json:"id"`
	State   string `json:"state"`
	Base    string `json:"artifact_base"`
	Output  string `json:"output"`
	Created string `json:"created"`
	Error   string `json:"error"`
}

func (i *Index) Jobs() ([]Job, error) {
	rows, e := i.db.Query(`SELECT id,state,artifact_base,output_path,created,error FROM jobs ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		var j Job
		if e = rows.Scan(&j.ID, &j.State, &j.Base, &j.Output, &j.Created, &j.Error); e != nil {
			return nil, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// WorkOne atomically claims a pending job. Running jobs are never automatically
// reclaimed: an observation timeout is not proof that another worker has stopped.
func (i *Index) WorkOne(ctx context.Context, progress func(string)) (bool, error) {
	var id int64
	var encoded []byte
	var base, out string
	e := i.db.QueryRowContext(ctx, `UPDATE jobs SET state='running',started=? WHERE id=(SELECT id FROM jobs WHERE state='pending' ORDER BY id LIMIT 1) AND state='pending' RETURNING id,lock_json,artifact_base,output_path`, time.Now().UTC().Format(time.RFC3339Nano)).Scan(&id, &encoded, &base, &out)
	if e == sql.ErrNoRows {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var l experiment.Lock
	if e = json.Unmarshal(encoded, &l); e == nil {
		_, e = experiment.Run(ctx, l, base, out, progress)
	}
	state, message := "done", ""
	if e != nil {
		state = "failed"
		message = e.Error()
	} else {
		e = i.AddRun(out)
		if e != nil {
			state = "failed"
			message = e.Error()
		}
	}
	_, updateErr := i.db.Exec(`UPDATE jobs SET state=?,finished=?,error=? WHERE id=? AND state='running'`, state, time.Now().UTC().Format(time.RFC3339Nano), message, id)
	if updateErr != nil {
		return true, fmt.Errorf("job %d result persistence: %w", id, updateErr)
	}
	return true, e
}
