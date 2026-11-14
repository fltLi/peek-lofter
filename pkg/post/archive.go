package post

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fltLi/peek-lofter/pkg/model"
)

// ArchiveInfo is one downloaded work archive found on disk. Err is set when
// the archive is unreadable or lacks its metadata.
type ArchiveInfo struct {
	Path string
	Post model.Post
	Err  error
}

const (
	metaEntry    = "meta.json"
	contentEntry = "content.html"
	imagesDir    = "images/"
)

// ArchivePath returns <out>/<blogId>/<postId>.zip. Both ids are required so
// downloaded works always live at stable, name-free paths.
func ArchivePath(out string, p model.Post) (string, error) {
	if p.Author.BlogID <= 0 {
		return "", errors.New("post: missing author id; cannot build id-based archive path")
	}
	if p.ID <= 0 {
		return "", errors.New("post: missing post id; cannot build id-based archive path")
	}
	return filepath.Join(out, strconv.FormatInt(p.Author.BlogID, 10), strconv.FormatInt(p.ID, 10)+".zip"), nil
}

// WriteArchive writes one work zip from a staging directory. The staging
// directory is expected to contain meta.json and content.html and optionally
// images/. Existing archives are skipped unless force is set.
func WriteArchive(out string, p model.Post, staging string, force bool) (string, bool, error) {
	target, err := ArchivePath(out, p)
	if err != nil {
		return "", false, err
	}
	if !force {
		if _, err := ValidateArchive(target, false); err == nil {
			return target, true, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", false, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".archive-*.zip")
	if err != nil {
		return "", false, err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	zw := zip.NewWriter(tmp)
	err = filepath.WalkDir(staging, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(staging, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		method := zip.Deflate
		if strings.HasPrefix(rel, imagesDir) {
			method = zip.Store
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		hdr.Name = rel
		hdr.Method = method
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(w, f)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		cleanup()
		return "", false, err
	}
	if err := zw.Close(); err != nil {
		cleanup()
		return "", false, err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", false, err
	}
	if force {
		_ = os.Remove(target)
	} else if _, err := os.Stat(target); err == nil {
		_ = os.Remove(target)
	}
	if err := os.Rename(tmpName, target); err != nil {
		cleanup()
		return "", false, err
	}
	return target, false, nil
}

// ReadMeta reads meta.json from a work archive.
func ReadMeta(path string) (model.Post, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return model.Post{}, err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != metaEntry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return model.Post{}, err
		}
		defer rc.Close()
		var p model.Post
		if err := json.NewDecoder(rc).Decode(&p); err != nil {
			return model.Post{}, err
		}
		return p, nil
	}
	return model.Post{}, fmt.Errorf("post: %s has no %s", path, metaEntry)
}

// ReadContent reads content.html from a work archive.
func ReadContent(path string) (string, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", err
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name != contentEntry {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		raw, err := io.ReadAll(rc)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	}
	return "", fmt.Errorf("post: %s has no %s", path, contentEntry)
}

// ReadArchive returns metadata and HTML from one archive.
func ReadArchive(path string) (model.Post, string, error) {
	p, err := ReadMeta(path)
	if err != nil {
		return model.Post{}, "", err
	}
	content, err := ReadContent(path)
	if err != nil {
		return model.Post{}, "", err
	}
	return p, content, nil
}

// ValidateArchive checks that an archive has meta.json and content.html.
// When withMedia is set, it also requires stored images when the metadata or
// the HTML references any.
func ValidateArchive(path string, withMedia bool) (model.Post, error) {
	p, err := ReadMeta(path)
	if err != nil {
		return model.Post{}, err
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		return model.Post{}, err
	}
	defer zr.Close()
	hasContent := false
	hasImage := false
	hasContentImage := false
	for _, f := range zr.File {
		switch {
		case f.Name == contentEntry:
			hasContent = true
			if withMedia {
				rc, err := f.Open()
				if err == nil {
					raw, readErr := io.ReadAll(rc)
					_ = rc.Close()
					if readErr == nil {
						hasContentImage = len(ExtractImageURLs(string(raw))) > 0
					}
				}
			}
		case strings.HasPrefix(f.Name, imagesDir) && !strings.HasSuffix(f.Name, "/"):
			hasImage = true
		}
	}
	if !hasContent {
		return model.Post{}, fmt.Errorf("post: %s has no %s", path, contentEntry)
	}
	if withMedia && (len(p.Images) > 0 || hasContentImage) && !hasImage {
		return model.Post{}, fmt.Errorf("post: %s has no images", path)
	}
	return p, nil
}

// ScanArchives walks root and returns every .zip archive. Corrupt archives are
// reported with Err so callers can warn and continue.
func ScanArchives(root string) ([]ArchiveInfo, error) {
	if root == "" {
		return nil, nil
	}
	var out []ArchiveInfo
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".zip") {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".archive-") {
			return nil
		}
		p, err := ReadMeta(path)
		out = append(out, ArchiveInfo{Path: path, Post: p, Err: err})
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return out, nil
}
