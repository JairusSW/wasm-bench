package storage

import (
	"context"
	"encoding/json"
	"github.com/wasmbench/wasmbench/experiment"
	"path/filepath"
	"sync"
	"testing"
)

func TestQueueClaimIsExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.sqlite")
	a, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	b, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	lock, _ := json.Marshal(experiment.Lock{})
	if _, e = a.db.Exec(`INSERT INTO jobs(lock_json,artifact_base,output_path,created) VALUES(?,?,?,?)`, lock, ".", "/invalid", "now"); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, db := range []*Index{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worked, _ := db.WorkOne(context.Background(), func(string) {})
			results <- worked
		}()
	}
	wg.Wait()
	close(results)
	claims := 0
	for worked := range results {
		if worked {
			claims++
		}
	}
	if claims != 1 {
		t.Fatalf("job claimed %d times", claims)
	}
	jobs, e := a.Jobs()
	if e != nil {
		t.Fatal(e)
	}
	if len(jobs) != 1 || jobs[0].State != "failed" || jobs[0].Error == "" {
		t.Fatal(jobs)
	}
}
