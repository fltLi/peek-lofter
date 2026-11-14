package cli

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// pageConfig controls the local browse pager. Pagination is never used by
// download or export commands.
type pageConfig struct {
	Page     int
	PageSize int
	PageSet  bool
	NoPager  bool
	TTY      bool
	In       io.Reader
	Out      io.Writer
	Prompt   io.Writer
	Header   string
}

func (c pageConfig) normalized() (page, size int) {
	page = c.Page
	if page < 1 {
		page = 1
	}
	size = c.PageSize
	if size <= 0 {
		size = 20
	}
	if size > 500 {
		size = 500
	}
	return page, size
}

// writePages shows rows either all at once or one page at a time.
func writePages(rows []string, cfg pageConfig) error {
	page, size := cfg.normalized()
	pages := (len(rows) + size - 1) / size
	if len(rows) == 0 {
		return nil
	}
	if cfg.PageSet {
		if page < 1 || page > pages {
			return fmt.Errorf("page %d out of range (1-%d)", page, pages)
		}
		return printPage(rows, page, size, pages, cfg.Out, cfg.Header)
	}
	if cfg.NoPager || !cfg.TTY {
		if cfg.Header != "" {
			if _, err := fmt.Fprintln(cfg.Out, cfg.Header); err != nil {
				return err
			}
		}
		for _, row := range rows {
			if _, err := fmt.Fprintln(cfg.Out, row); err != nil {
				return err
			}
		}
		return nil
	}
	if cfg.In == nil {
		return printPage(rows, 1, size, pages, cfg.Out, cfg.Header)
	}
	reader := bufio.NewReader(cfg.In)
	for {
		if err := printPage(rows, page, size, pages, cfg.Out, cfg.Header); err != nil {
			return err
		}
		if _, err := fmt.Fprint(cfg.Prompt, "  [n]ext  [p]rev  [q]uit: "); err != nil {
			return err
		}
		line, err := reader.ReadString('\n')
		if err != nil && len(strings.TrimSpace(line)) == 0 {
			return nil
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "", "n", "next":
			if page < pages {
				page++
			}
		case "p", "prev":
			if page > 1 {
				page--
			}
		case "q", "quit":
			return nil
		default:
			n, err := strconv.Atoi(strings.TrimSpace(line))
			if err == nil && n >= 1 && n <= pages {
				page = n
			}
		}
	}
}

// printPage writes one page and repeats the header.
func printPage(rows []string, page, size, pages int, out io.Writer, header string) error {
	start := (page - 1) * size
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	if _, err := fmt.Fprintf(
		out,
		"page %d/%d  items %d-%d of %d\n",
		page,
		pages,
		start+1,
		end,
		len(rows),
	); err != nil {
		return err
	}
	if header != "" {
		if _, err := fmt.Fprintln(out, header); err != nil {
			return err
		}
	}
	for _, row := range rows[start:end] {
		if _, err := fmt.Fprintln(out, row); err != nil {
			return err
		}
	}
	return nil
}
