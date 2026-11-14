package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/fltLi/peek-lofter/pkg/model"
	"github.com/fltLi/peek-lofter/pkg/post"
	"github.com/fltLi/peek-lofter/pkg/tag"
)

// liveLine renders one carriage-return status line on a terminal.
type liveLine struct {
	w io.Writer
	n int
}

// Set replaces the current status line and pads the previous one.
func (l *liveLine) Set(line string) {
	if l.w == nil {
		return
	}
	pad := ""
	if len(line) < l.n {
		pad = strings.Repeat(" ", l.n-len(line))
	}
	fmt.Fprintf(l.w, "\r%s%s", line, pad)
	if len(line) > l.n {
		l.n = len(line)
	}
}

// Clear erases the current status line.
func (l *liveLine) Clear() {
	if l.w == nil || l.n == 0 {
		return
	}
	fmt.Fprintf(l.w, "\r%s\r", strings.Repeat(" ", l.n))
	l.n = 0
}

type tagProgressOptions struct {
	Out     io.Writer
	TTY     bool
	Quiet   bool
	Verbose bool
	Since   time.Time
	Until   time.Time
	Tags    []string
}

// tagProgress reports date coverage and post counts, not internal window details.
type tagProgress struct {
	out     io.Writer
	tty     bool
	quiet   bool
	verbose bool
	since   time.Time
	until   time.Time
	tags    []string
	start   time.Time
	now     func() time.Time

	mu           sync.Mutex
	live         liveLine
	current      tag.Window
	windowPosts  int
	posts        int
	coveredStart time.Time
	coveredEnd   time.Time
	started      bool
	lastBeat     time.Time
	lastRender   time.Time
	stop         chan struct{}
	wg           sync.WaitGroup
}

func newTagProgress(opt tagProgressOptions) *tagProgress {
	return &tagProgress{
		out:        opt.Out,
		tty:        opt.TTY,
		quiet:      opt.Quiet,
		verbose:    opt.Verbose,
		since:      opt.Since,
		until:      opt.Until,
		tags:       opt.Tags,
		start:      time.Now(),
		now:        time.Now,
		live:       liveLine{w: opt.Out},
		coveredEnd: opt.Until,
	}
}

// Window implements tag.Progress.
func (p *tagProgress) Window(w tag.Window) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current = w
	p.windowPosts = 0
	if !p.started {
		p.started = true
		p.lastBeat = p.now()
		p.lastRender = p.lastBeat
		if !p.tty || p.verbose {
			fmt.Fprintf(p.out, "tag %s  requested %s\n", p.labelLocked(), p.requestedLocked())
		}
	}
	p.startTickerLocked()
	if p.tty && !p.verbose {
		p.renderLocked(true)
	}
}

// Ref implements tag.Progress.
func (p *tagProgress) Ref(model.Ref) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.posts++
	p.windowPosts++
	if p.tty && !p.verbose {
		p.renderLocked(false)
		return
	}
	if !p.tty && !p.verbose {
		p.heartbeatLocked(false)
	}
}

// WindowDone implements tag.Progress.
func (p *tagProgress) WindowDone(w tag.Window, res tag.WindowResult) {
	if p.quiet {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	switch res.Status {
	case "complete", "partial", "empty":
		if p.coveredStart.IsZero() || w.Start.Before(p.coveredStart) {
			p.coveredStart = w.Start
		}
	}
	if p.verbose {
		fmt.Fprintln(p.out, p.periodLineLocked(w, res))
		return
	}
	if p.tty {
		p.renderLocked(true)
		return
	}
	p.heartbeatLocked(false)
}

// Done implements tag.Progress. The command prints the final summary.
func (p *tagProgress) Done(tag.Summary) {
	if p.quiet {
		return
	}
	p.stopTicker()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.live.Clear()
}

// Clear removes any live line left by an early error.
func (p *tagProgress) Clear() {
	p.stopTicker()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.live.Clear()
}

func (p *tagProgress) startTickerLocked() {
	if p.quiet || p.verbose || p.stop != nil {
		return
	}
	stop := make(chan struct{})
	p.stop = stop
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				p.tick()
			case <-stop:
				return
			}
		}
	}()
}

func (p *tagProgress) stopTicker() {
	p.mu.Lock()
	stop := p.stop
	p.stop = nil
	p.mu.Unlock()
	if stop != nil {
		close(stop)
		p.wg.Wait()
	}
}

func (p *tagProgress) tick() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.tty {
		p.renderLocked(true)
		return
	}
	p.heartbeatLocked(true)
}

// Coverage returns the oldest and newest dates processed.
func (p *tagProgress) Coverage() (time.Time, time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.coveredStart, p.coveredEnd
}

// Elapsed returns the time since the progress display started.
func (p *tagProgress) Elapsed() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.now().Sub(p.start)
}

func (p *tagProgress) labelLocked() string {
	name := p.current.Tag
	if name == "" && len(p.tags) > 0 {
		name = p.tags[0]
	}
	if len(p.tags) > 1 {
		return fmt.Sprintf("%s (+%d)", name, len(p.tags)-1)
	}
	return name
}

func (p *tagProgress) requestedLocked() string {
	if p.since.IsZero() {
		return "auto (up to 10 years)"
	}
	return fmt.Sprintf("%s..%s", p.since.Format(time.DateOnly), p.until.Format(time.DateOnly))
}

func (p *tagProgress) renderLocked(force bool) {
	if !p.tty || p.verbose || p.quiet {
		return
	}
	now := p.now()
	if !force && now.Sub(p.lastRender) < 100*time.Millisecond {
		return
	}
	p.lastRender = now
	p.live.Set(p.statusLineLocked(now))
}

func (p *tagProgress) heartbeatLocked(force bool) {
	if p.tty || p.verbose || p.quiet {
		return
	}
	now := p.now()
	if !force && now.Sub(p.lastBeat) < 2*time.Second {
		return
	}
	p.lastBeat = now
	fmt.Fprintf(p.out, "[%s] %s\n", clock(p.now().Sub(p.start)), p.statusLineLocked(now))
}

func (p *tagProgress) statusLineLocked(now time.Time) string {
	name := p.labelLocked()
	posts := fmt.Sprintf("posts %d", p.posts)
	if p.coveredStart.IsZero() {
		return fmt.Sprintf("%s  completed none  %s  %s", name, posts, shortDuration(now.Sub(p.start)))
	}
	covered := fmt.Sprintf("completed %s..%s", p.coveredStart.Format(time.DateOnly), p.until.Format(time.DateOnly))
	percent := ""
	if !p.since.IsZero() && p.until.After(p.since) {
		done := p.until.Sub(p.coveredStart)
		total := p.until.Sub(p.since)
		pct := int(done * 100 / total)
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		percent = fmt.Sprintf("%d%%  ", pct)
	}
	return fmt.Sprintf("%s  %s%s  %s  %s", name, percent, covered, posts, shortDuration(now.Sub(p.start)))
}

func (p *tagProgress) periodLineLocked(w tag.Window, res tag.WindowResult) string {
	status := res.Status
	switch status {
	case "suspect", "unresolved":
		status = "incomplete"
	case "failed":
		status = "failed"
	}
	line := fmt.Sprintf(
		"%s..%s  %-10s  %s",
		w.Start.Format(time.DateOnly),
		w.End.Add(-time.Second).Format(time.DateOnly),
		status,
		formatPosts(p.windowPosts),
	)
	if res.Err != nil {
		line += "  " + res.Err.Error()
	}
	return line
}

func formatPosts(n int) string {
	if n == 1 {
		return "1 post"
	}
	return fmt.Sprintf("%d posts", n)
}

func shortDuration(d time.Duration) string {
	if d < time.Second {
		return "0s"
	}
	return d.Round(time.Second).String()
}

func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	h := int(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := int(d / time.Second)
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// postReport keeps a batch download to one summary and a bounded error list.
type postReport struct {
	out     io.Writer
	tty     bool
	quiet   bool
	verbose bool
	total   int
	start   time.Time
	now     func() time.Time

	mu         sync.Mutex
	done       int
	saved      int
	skipped    int
	failed     int
	errorLines []string
	live       liveLine
	lastBeat   time.Time
	lastRender time.Time
	stop       chan struct{}
	wg         sync.WaitGroup
}

func newPostReport(out io.Writer, tty, quiet, verbose bool, total int) *postReport {
	now := time.Now()
	return &postReport{
		out:        out,
		tty:        tty,
		quiet:      quiet,
		verbose:    verbose,
		total:      total,
		start:      now,
		now:        time.Now,
		live:       liveLine{w: out},
		lastBeat:   now,
		lastRender: now,
	}
}

// Result records one completed batch item.
func (r *postReport) Result(res post.Result) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done++
	switch res.Status {
	case "failed":
		r.failed++
		line := fmt.Sprintf("%s  %v", res.Ref.URL, res.Err)
		if r.verbose {
			fmt.Fprintf(r.out, "error  %s\n", line)
		} else if len(r.errorLines) < 5 {
			r.errorLines = append(r.errorLines, line)
		}
	case "skipped":
		r.skipped++
		if r.verbose {
			fmt.Fprintf(r.out, "skipped  %s\n", res.Path)
		}
	default:
		r.saved++
		if r.verbose {
			fmt.Fprintf(r.out, "saved  %s\n", res.Path)
		}
	}
	r.startTickerLocked()
	if r.tty && !r.quiet && !r.verbose {
		r.renderLocked(false)
		return
	}
	if !r.tty && !r.quiet && !r.verbose {
		r.heartbeatLocked(false)
	}
}

// Summary prints the aggregate result after all items finish.
func (r *postReport) Summary(sum post.Summary) {
	r.stopTicker()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live.Clear()
	r.printErrorsLocked()
	if r.quiet {
		return
	}
	fmt.Fprintln(r.out, "post")
	fmt.Fprintf(r.out, "  total    %d\n", sum.Total)
	fmt.Fprintf(r.out, "  saved    %d\n", sum.Saved)
	fmt.Fprintf(r.out, "  skipped  %d\n", sum.Skipped)
	fmt.Fprintf(r.out, "  failed   %d\n", sum.Failed)
	fmt.Fprintf(r.out, "  elapsed  %s\n", shortDuration(r.now().Sub(r.start)))
}

// Clear removes any live line left by an early error.
func (r *postReport) Clear() {
	r.stopTicker()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.live.Clear()
}

func (r *postReport) startTickerLocked() {
	if r.quiet || r.verbose || r.stop != nil {
		return
	}
	stop := make(chan struct{})
	r.stop = stop
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				r.tick()
			case <-stop:
				return
			}
		}
	}()
}

func (r *postReport) stopTicker() {
	r.mu.Lock()
	stop := r.stop
	r.stop = nil
	r.mu.Unlock()
	if stop != nil {
		close(stop)
		r.wg.Wait()
	}
}

func (r *postReport) tick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tty {
		r.renderLocked(true)
		return
	}
	r.heartbeatLocked(true)
}

func (r *postReport) printErrorsLocked() {
	if len(r.errorLines) == 0 {
		return
	}
	fmt.Fprintln(r.out, "failures")
	for _, line := range r.errorLines {
		fmt.Fprintf(r.out, "  %s\n", line)
	}
	if r.failed > len(r.errorLines) {
		fmt.Fprintf(r.out, "  (%d more; use --verbose)\n", r.failed-len(r.errorLines))
	}
}

func (r *postReport) renderLocked(force bool) {
	if !r.tty || r.quiet || r.verbose {
		return
	}
	now := r.now()
	if !force && now.Sub(r.lastRender) < 100*time.Millisecond {
		return
	}
	r.lastRender = now
	r.live.Set(r.statusLineLocked(now))
}

func (r *postReport) heartbeatLocked(force bool) {
	if r.tty || r.quiet || r.verbose {
		return
	}
	now := r.now()
	if !force && now.Sub(r.lastBeat) < 2*time.Second {
		return
	}
	r.lastBeat = now
	fmt.Fprintf(r.out, "[%s] %s\n", clock(now.Sub(r.start)), r.statusLineLocked(now))
}

func (r *postReport) statusLineLocked(now time.Time) string {
	return fmt.Sprintf(
		"post  %d/%d  saved %d  skipped %d  failed %d  %s",
		r.done,
		r.total,
		r.saved,
		r.skipped,
		r.failed,
		shortDuration(now.Sub(r.start)),
	)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
