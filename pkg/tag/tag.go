package tag

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/store"
)

// Source is the subset of the LOFTER client used by the tag runner.
type Source interface {
	TagPage(ctx context.Context, q model.TagPageQuery) (model.TagPage, error)
}

// Sink persists refs and window evidence and reports previously completed windows.
type Sink interface {
	AppendRef(model.Ref) error
	AppendWindow(store.WindowEvent) error
	Completed(window string) bool
}

// WindowResult is the terminal outcome of one window.
type WindowResult struct {
	Status string
	Err    error
}

// Progress receives crawl events for human-readable progress.
type Progress interface {
	Window(Window)
	Ref(model.Ref)
	WindowDone(Window, WindowResult)
	Done(Summary)
}

// NopProgress discards all progress events.
type NopProgress struct{}

func (NopProgress) Window(Window)                   {}
func (NopProgress) Ref(model.Ref)                   {}
func (NopProgress) WindowDone(Window, WindowResult) {}
func (NopProgress) Done(Summary)                    {}

// Options controls tag planning: tags, date range and type filter.
type Options struct {
	Tags  []string
	Since time.Time
	Until time.Time
	Types string
}

// Window is one calendar-month time slice with its own cold cache key.
type Window struct {
	ID     string
	Tag    string
	Start  time.Time
	End    time.Time
	PostYm string
	Kind   string
	Key    string
}

// Summary reports how many windows and refs a crawl produced.
type Summary struct {
	Windows        int
	Complete       int
	Partial        int
	Empty          int
	Suspect        int
	Unresolved     int
	Refs           int
	StoppedByLimit bool
	StoppedByEmpty bool
}

// PartialError means some windows are suspect or unresolved: data is usable but incomplete.
type PartialError struct {
	Summary Summary
}

// Error implements the error interface.
func (e *PartialError) Error() string {
	return fmt.Sprintf(
		"tag crawl partial: suspect=%d unresolved=%d",
		e.Summary.Suspect,
		e.Summary.Unresolved,
	)
}

// Planner turns tags and a date range into windows.
type Planner struct {
	opts Options
}

// NewPlanner validates options and applies defaults.
func NewPlanner(opts Options) (*Planner, error) {
	if len(opts.Tags) == 0 {
		return nil, errors.New("tag: at least one tag is required")
	}
	var tags []string
	for _, raw := range opts.Tags {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				tags = append(tags, part)
			}
		}
	}
	opts.Tags = tags
	if opts.Types == "" {
		opts.Types = "1,2,4"
	}
	return &Planner{opts: opts}, nil
}

// Plan returns windows newest-first. Every window gets a distinct cold key so a
// cached response is not reused across months.
func (p *Planner) Plan() ([]Window, error) {
	until := p.opts.Until
	if until.IsZero() {
		until = time.Now()
	}
	since := p.opts.Since
	if since.IsZero() {
		since = until.AddDate(-10, 0, 0)
	}
	if !since.Before(until) {
		return nil, fmt.Errorf(
			"tag: since %s must be before until %s",
			since.Format(time.DateOnly),
			until.Format(time.DateOnly),
		)
	}
	var windows []Window
	index := 0
	for _, name := range p.opts.Tags {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		end := until
		for end.After(since) {
			start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())
			if !start.Before(end) {
				start = start.AddDate(0, -1, 0)
			}
			if start.Before(since) {
				start = since
			}
			if !start.Before(end) {
				break
			}
			index++
			windows = append(windows, Window{
				ID:     windowID(name, start, end),
				Tag:    name,
				Start:  start,
				End:    end,
				PostYm: start.Format("200601"),
				Kind:   "month",
				Key:    coldKey(p.opts.Types, index),
			})
			end = start
		}
	}
	return windows, nil
}

// windowID is stable for a given tag and half-open time range.
func windowID(tag string, start, end time.Time) string {
	return fmt.Sprintf("%s:%s-%s", tag, start.Format("20060102T1504"), end.Add(-time.Second).Format("20060102T1504"))
}

// coldKey appends empty type tokens to give each month a distinct cache key.
func coldKey(base string, n int) string {
	if n < 1 {
		n = 1
	}
	return base + strings.Repeat(",", n)
}

// nextKey forces another cache entry when a window returns too many out-of-range items.
func nextKey(key string) string {
	return key + ","
}

// Runner executes a window plan against a Source and optional Sink.
// StopAfterEmpty is measured in consecutive empty months.
type Runner struct {
	Source         Source
	Sink           Sink
	Progress       Progress
	Limit          int
	StopAfterEmpty int
}

// Run crawls windows sequentially, emits new refs and returns a summary.
// A PartialError means the crawl completed with suspect or unresolved windows.
func (r *Runner) Run(ctx context.Context, windows []Window, emit func(model.Ref) error) (Summary, error) {
	if r.Source == nil {
		return Summary{}, errors.New("tag: nil source")
	}
	if r.Progress == nil {
		r.Progress = NopProgress{}
	}
	if emit == nil {
		emit = func(model.Ref) error { return nil }
	}
	var sum Summary
	sum.Windows = len(windows)
	seen := map[string]bool{}
	consecutiveEmpty := 0
	for _, w := range windows {
		if err := ctx.Err(); err != nil {
			return sum, err
		}
		if r.Sink != nil && r.Sink.Completed(w.ID) {
			continue
		}
		r.Progress.Window(w)
		ev, limited, err := r.crawlOne(ctx, w, emit, seen)
		if err != nil {
			r.Progress.WindowDone(w, WindowResult{Status: "failed", Err: err})
			return sum, err
		}
		r.Progress.WindowDone(w, WindowResult{Status: ev.Status})
		if r.Sink != nil {
			if err := r.Sink.AppendWindow(ev); err != nil {
				return sum, err
			}
		}
		switch ev.Status {
		case "complete":
			sum.Complete++
		case "partial":
			sum.Partial++
		case "empty":
			sum.Empty++
		case "suspect":
			sum.Suspect++
		case "unresolved":
			sum.Unresolved++
		}
		if ev.Inside == 0 {
			consecutiveEmpty++
		} else {
			consecutiveEmpty = 0
		}
		if limited {
			sum.StoppedByLimit = true
			break
		}
		if r.StopAfterEmpty > 0 && consecutiveEmpty >= r.StopAfterEmpty {
			sum.StoppedByEmpty = true
			break
		}
	}
	sum.Refs = len(seen)
	r.Progress.Done(sum)
	if sum.Suspect > 0 || sum.Unresolved > 0 {
		return sum, &PartialError{Summary: sum}
	}
	return sum, nil
}

func (r *Runner) crawlOne(
	ctx context.Context,
	w Window,
	emit func(model.Ref) error,
	seen map[string]bool,
) (store.WindowEvent, bool, error) {
	started := time.Now()
	ev := store.WindowEvent{Window: w.ID, Tag: w.Tag, Kind: w.Kind, Key: w.Key, StartedAt: started}
	key := w.Key
	for attempt := 1; attempt <= 3; attempt++ {
		ev.Key = key
		offset := 0
		pages := 0
		items := 0
		inside := 0
		outside := 0
		capHit := false
		limited := false
	pageLoop:
		for {
			q := model.TagPageQuery{
				Tag:       w.Tag,
				Type:      "total",
				PostTypes: key,
				PostYm:    w.PostYm,
				StartMs:   w.Start.UnixMilli(),
				EndMs:     w.End.UnixMilli(),
				Offset:    offset,
			}
			page, err := r.Source.TagPage(ctx, q)
			if err != nil {
				return ev, false, err
			}
			pages++
			items += len(page.Refs)
			if len(page.Refs) == 0 {
				break
			}
			for _, ref := range page.Refs {
				ts := ref.PublishedAt.Time
				if ts.Before(w.Start) || !ts.Before(w.End) {
					outside++
					continue
				}
				inside++
				keyID := strconv.FormatInt(ref.ID, 10)
				if ref.ID == 0 {
					keyID = ref.URL
				}
				if seen[keyID] {
					continue
				}
				seen[keyID] = true
				ref.Source = &model.Source{Kind: "tag", Name: w.Tag, Window: w.ID}
				if r.Sink != nil {
					if err := r.Sink.AppendRef(ref); err != nil {
						return ev, false, err
					}
				}
				if err := emit(ref); err != nil {
					return ev, false, err
				}
				r.Progress.Ref(ref)
				if r.Limit > 0 && len(seen) >= r.Limit {
					limited = true
					break pageLoop
				}
			}
			next := page.Next
			if next == -1 {
				break
			}
			if next >= 1010 {
				capHit = true
				break
			}
			if next <= offset {
				break
			}
			offset = next
		}
		ev.Pages = pages
		ev.Items = items
		ev.Inside = inside
		ev.Outside = outside
		if limited {
			ev.EndedAt = time.Now()
			ev.Status = "partial"
			return ev, true, nil
		}
		// The tag API caps a month at about 1010 items. Date filters are
		// ignored, so splitting into days cannot narrow the result.
		if capHit {
			ev.EndedAt = time.Now()
			ev.Status = "unresolved"
			return ev, false, nil
		}
		// Too many out-of-window items means the cache ignored the filter.
		// Retry with a fresh key before accepting a partial slice.
		if outside > 0 && (inside == 0 || outside*3 > inside) && attempt < 3 {
			key = nextKey(key)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		ev.EndedAt = time.Now()
		switch {
		case inside == 0:
			ev.Status = "empty"
		case outside > 0:
			ev.Status = "partial"
		default:
			ev.Status = "complete"
		}
		return ev, false, nil
	}
	ev.EndedAt = time.Now()
	ev.Status = "suspect"
	return ev, false, nil
}
