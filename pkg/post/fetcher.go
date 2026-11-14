package post

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/fltLi/peek-lofter/pkg/lofter"
	"github.com/fltLi/peek-lofter/pkg/model"
)

// DetailSource fetches a post by reference.
type DetailSource interface {
	PostDetail(ctx context.Context, ref model.Ref, mode lofter.DetailMode) (model.Post, error)
}

// Media downloads one image into a staging directory.
type Media interface {
	Download(ctx context.Context, img model.Image, dir, base string) (string, error)
}

// Options controls post fetching and archive writing.
type Options struct {
	Out         string
	WithMedia   bool
	WithVideo   bool
	Force       bool
	Concurrency int
}

// Result is one post outcome for batch callers.
type Result struct {
	Ref    model.Ref
	Post   model.Post
	Path   string
	Status string
	Err    error
}

// Summary aggregates a batch of post results.
type Summary struct {
	Total   int
	Saved   int
	Skipped int
	Failed  int
}

// Fetcher combines a detail source and a media downloader.
type Fetcher struct {
	Source DetailSource
	Media  Media
}

// Fetch obtains a full post. Archives always use full detail.
func (f *Fetcher) Fetch(ctx context.Context, ref model.Ref) (model.Post, error) {
	p, err := f.Source.PostDetail(ctx, ref, lofter.DetailFull)
	if err != nil {
		return model.Post{}, err
	}
	if p.Source == nil {
		p.Source = ref.Source
	}
	return p, nil
}

// Save writes one work zip atomically. Existing archives are skipped unless
// Force is set.
func (f *Fetcher) Save(ctx context.Context, p model.Post, opt Options) (string, bool, error) {
	if opt.Out == "" {
		opt.Out = "peek-out"
	}
	forceWrite := opt.Force
	if !opt.Force {
		if target, err := ArchivePath(opt.Out, p); err == nil {
			if _, err := ValidateArchive(target, opt.WithMedia); err == nil {
				return target, true, nil
			}
			if opt.WithMedia {
				if _, err := os.Stat(target); err == nil {
					forceWrite = true
				}
			}
		}
	}
	staging, err := os.MkdirTemp("", "peek-lofter-archive-*")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(staging)

	meta := p
	meta.Content = ""
	rawMeta, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", false, err
	}
	if err := os.WriteFile(filepath.Join(staging, metaEntry), rawMeta, 0o644); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(filepath.Join(staging, contentEntry), []byte(p.Content), 0o644); err != nil {
		return "", false, err
	}

	if opt.WithMedia {
		images := collectImages(p)
		if len(images) > 0 {
			if f.Media == nil {
				return "", false, errors.New("post: media downloader is nil")
			}
			imgDir := filepath.Join(staging, "images")
			for i, img := range images {
				base := fmt.Sprintf("%03d", i+1)
				if _, err := f.Media.Download(ctx, img, imgDir, base); err != nil {
					return "", false, err
				}
			}
		}
	}

	path, skipped, err := WriteArchive(opt.Out, p, staging, forceWrite)
	if err != nil {
		return "", false, err
	}
	return path, skipped, nil
}

// FetchAll dedupes refs, skips existing archives before any request, and runs
// a bounded worker pool. Errors do not abort the rest of the batch.
func (f *Fetcher) FetchAll(
	ctx context.Context,
	refs []model.Ref,
	opt Options,
	emit func(Result) error,
) (Summary, error) {
	if opt.Out == "" {
		opt.Out = "peek-out"
	}
	if opt.Concurrency <= 0 {
		opt.Concurrency = 2
	}
	if emit == nil {
		emit = func(Result) error { return nil }
	}
	seen := map[string]bool{}
	unique := make([]model.Ref, 0, len(refs))
	for _, ref := range refs {
		key := RefKey(ref)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, ref)
	}

	var existing existingIndex
	if !opt.Force {
		existing = loadExisting(opt.Out)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan model.Ref)
	results := make(chan Result, opt.Concurrency)
	var wg sync.WaitGroup
	for range opt.Concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ref := range jobs {
				if path, p, ok := existing.find(ref, opt.WithMedia); ok {
					select {
					case results <- Result{Ref: ref, Post: p, Path: path, Status: "skipped"}:
					case <-ctx.Done():
					}
					continue
				}
				p, err := f.Fetch(ctx, ref)
				res := Result{Ref: ref, Post: p, Err: err}
				if err == nil {
					path, skipped, saveErr := f.Save(ctx, p, opt)
					res.Path = path
					if saveErr != nil {
						res.Err = saveErr
					} else if skipped {
						res.Status = "skipped"
					} else {
						res.Status = "saved"
					}
				}
				if res.Err != nil {
					res.Status = "failed"
				}
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, ref := range unique {
			select {
			case jobs <- ref:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var sum Summary
	sum.Total = len(unique)
	for res := range results {
		switch res.Status {
		case "saved":
			sum.Saved++
		case "skipped":
			sum.Skipped++
		case "failed":
			sum.Failed++
		}
		if err := emit(res); err != nil {
			cancel()
			return sum, err
		}
	}
	return sum, nil
}

// RefKey returns the stable dedupe key for a ref.
func RefKey(ref model.Ref) string {
	if ref.ID > 0 {
		return "id:" + strconv.FormatInt(ref.ID, 10)
	}
	if ref.URL != "" {
		return "url:" + NormalizeURL(ref.URL)
	}
	if ref.Permalink != "" {
		return "permalink:" + ref.Author.BlogName + ":" + ref.Permalink
	}
	return ""
}

type existingIndex struct {
	byID        map[int64]ArchiveInfo
	byURL       map[string]ArchiveInfo
	byPermalink map[string]ArchiveInfo
}

func loadExisting(out string) existingIndex {
	idx := existingIndex{
		byID:        map[int64]ArchiveInfo{},
		byURL:       map[string]ArchiveInfo{},
		byPermalink: map[string]ArchiveInfo{},
	}
	if out == "" {
		return idx
	}
	items, err := ScanArchives(out)
	if err != nil {
		return idx
	}
	for _, item := range items {
		if item.Err != nil {
			continue
		}
		p := item.Post
		if p.ID > 0 {
			idx.byID[p.ID] = item
		}
		postURL := p.URL
		if postURL == "" && p.Permalink != "" {
			postURL = model.PostURL(p.Author.BlogName, p.Permalink)
		}
		if postURL != "" {
			idx.byURL[NormalizeURL(postURL)] = item
		}
		if p.Permalink != "" {
			idx.byPermalink[p.Author.BlogName+":"+p.Permalink] = item
		}
	}
	return idx
}

func (idx existingIndex) find(ref model.Ref, withMedia bool) (string, model.Post, bool) {
	var item ArchiveInfo
	var ok bool
	if ref.ID > 0 {
		item, ok = idx.byID[ref.ID]
	}
	if !ok && ref.URL != "" {
		item, ok = idx.byURL[NormalizeURL(ref.URL)]
	}
	if !ok && ref.Permalink != "" {
		item, ok = idx.byPermalink[ref.Author.BlogName+":"+ref.Permalink]
	}
	if !ok {
		return "", model.Post{}, false
	}
	if _, err := ValidateArchive(item.Path, withMedia); err != nil {
		return "", model.Post{}, false
	}
	return item.Path, item.Post, true
}

// collectImages merges photoLinks with images embedded in the content HTML.
func collectImages(p model.Post) []model.Image {
	var out []model.Image
	seen := map[string]bool{}
	add := func(img model.Image) {
		if (img.Raw == "" && img.Orign == "") || seen[img.Raw+"|"+img.Orign] {
			return
		}
		seen[img.Raw+"|"+img.Orign] = true
		out = append(out, img)
	}
	for _, img := range p.Images {
		add(img)
	}
	for _, u := range ExtractImageURLs(p.Content) {
		add(model.Image{Raw: u})
	}
	return out
}
