package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// WindowEvent is persisted evidence for one crawl window.
type WindowEvent struct {
	Window    string    `json:"window"`
	Tag       string    `json:"tag"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
	Key       string    `json:"key,omitempty"`
	Pages     int       `json:"pages"`
	Items     int       `json:"items"`
	Inside    int       `json:"inside"`
	Outside   int       `json:"outside"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"startedAt"`
	EndedAt   time.Time `json:"endedAt"`
}

// State is the resumable cursor stored next to a dataset.
type State struct {
	Queue    []string  `json:"queue,omitempty"`
	Current  string    `json:"current,omitempty"`
	Offset   int       `json:"offset,omitempty"`
	LastSync time.Time `json:"lastSync,omitempty"`
}

// Manifest describes dataset parameters and counters.
type Manifest struct {
	Version   int       `json:"version"`
	Tags      []string  `json:"tags"`
	Since     time.Time `json:"since,omitempty"`
	Until     time.Time `json:"until,omitempty"`
	Types     string    `json:"types,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	LastRunAt time.Time `json:"lastRunAt,omitempty"`
	Works     int       `json:"works"`
}

// Dataset is an append-only tag crawl directory. Only one process should
// write a given directory at a time.
type Dataset struct {
	Dir string

	mu        sync.Mutex
	refs      *os.File
	windows   *os.File
	completed map[string]bool
	manifest  Manifest
}

// Open creates or opens a dataset and loads previously completed windows.
func Open(dir string) (*Dataset, error) {
	if dir == "" {
		return nil, errors.New("store: empty directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	refs, err := os.OpenFile(filepath.Join(dir, "refs.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	windows, err := os.OpenFile(filepath.Join(dir, "windows.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		_ = refs.Close()
		return nil, err
	}
	d := &Dataset{
		Dir:       dir,
		refs:      refs,
		windows:   windows,
		completed: map[string]bool{},
		manifest:  Manifest{Version: 1, CreatedAt: time.Now()},
	}
	d.loadCompleted()
	d.loadManifest()
	return d, nil
}

func (d *Dataset) loadCompleted() {
	f, err := os.Open(filepath.Join(d.Dir, "windows.jsonl"))
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 1<<20)
	for sc.Scan() {
		var ev WindowEvent
		if err := json.Unmarshal(sc.Bytes(), &ev); err == nil && ev.Status == "complete" {
			d.completed[ev.Window] = true
		}
	}
}

func (d *Dataset) loadManifest() {
	raw, err := os.ReadFile(filepath.Join(d.Dir, "manifest.json"))
	if err != nil {
		return
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err == nil {
		d.manifest = m
	}
}

// AppendRef appends one ref and increments the in-memory work counter.
func (d *Dataset) AppendRef(ref model.Ref) error {
	line, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.refs.Write(append(line, '\n')); err != nil {
		return err
	}
	d.manifest.Works++
	return nil
}

// AppendWindow appends window evidence and remembers complete windows.
func (d *Dataset) AppendWindow(ev WindowEvent) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := d.windows.Write(append(line, '\n')); err != nil {
		return err
	}
	if ev.Status == "complete" {
		d.completed[ev.Window] = true
	}
	return nil
}

// Completed reports whether a window completed in a previous run.
func (d *Dataset) Completed(window string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.completed[window]
}

// ReadRefs iterates unique refs from refs.jsonl in file order.
func (d *Dataset) ReadRefs(fn func(model.Ref) error) error {
	f, err := os.Open(filepath.Join(d.Dir, "refs.jsonl"))
	if err != nil {
		return err
	}
	defer f.Close()
	seen := map[int64]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024), 4<<20)
	for sc.Scan() {
		var ref model.Ref
		if err := json.Unmarshal(sc.Bytes(), &ref); err != nil {
			continue
		}
		if seen[ref.ID] {
			continue
		}
		seen[ref.ID] = true
		if err := fn(ref); err != nil {
			return err
		}
	}
	return sc.Err()
}

// LoadState reads state.json, returning a zero State if it does not exist.
func (d *Dataset) LoadState() (State, error) {
	var st State
	raw, err := os.ReadFile(filepath.Join(d.Dir, "state.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	return st, json.Unmarshal(raw, &st)
}

// SaveState atomically replaces state.json.
func (d *Dataset) SaveState(st State) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(d.Dir, "state.json"), raw, 0o644)
}

// SaveManifest atomically writes manifest.json with a fresh lastRunAt.
func (d *Dataset) SaveManifest() error {
	d.mu.Lock()
	d.manifest.LastRunAt = time.Now()
	m := d.manifest
	d.mu.Unlock()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(d.Dir, "manifest.json"), raw, 0o644)
}

// Configure records the crawl parameters before the first run.
func (d *Dataset) Configure(tags []string, since, until time.Time, types string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.manifest.Tags = tags
	d.manifest.Since = since
	d.manifest.Until = until
	d.manifest.Types = types
}

// Manifest returns a copy of the in-memory manifest.
func (d *Dataset) Manifest() Manifest {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.manifest
}

// Close flushes and closes the append-only files.
func (d *Dataset) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	var first error
	if d.refs != nil {
		if err := d.refs.Close(); err != nil && first == nil {
			first = err
		}
	}
	if d.windows != nil {
		if err := d.windows.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// writeAtomic writes a temp file and renames it. On Windows a direct rename
// over an existing file can fail, so the target is removed once and retried.
func writeAtomic(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			return fmt.Errorf("rename %s: %w", path, err2)
		}
	}
	return nil
}
