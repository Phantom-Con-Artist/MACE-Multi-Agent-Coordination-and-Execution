package blobs

import (
	"archive/tar"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// A shared folder travels as one file: a tar of its regular files and
// folders, chunked and addressed like any other. Unpacking accepts only
// plain files and folders inside the destination, so a package can never
// write elsewhere, link to anything, or set special permissions.

// FolderType marks a manifest whose content is a packed folder.
const FolderType = "application/x-mace-folder"

// Folder limits keep one share from filling a disk by accident.
const (
	MaxFolderFiles = 20000
	MaxFolderBytes = 2 << 30
)

// skipped names are rebuilt or private, never worth moving.
var skipped = map[string]bool{".git": true, "node_modules": true, ".venv": true, "__pycache__": true, "graphify-out": true}

// AddFolder packs the folder dir, read through fsys, and stores it.
func (s *Store) AddFolder(fsys fs.FS, dir, name string) (Manifest, error) {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(packFolder(fsys, dir, pw))
	}()
	m, err := s.Add(pr, name, FolderType)
	pr.CloseWithError(err)
	return m, err
}

func packFolder(fsys fs.FS, dir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	files, total := 0, int64(0)
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p != dir && skipped[d.Name()] {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, dir), "/")
		if rel == "" {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return tw.WriteHeader(&tar.Header{Name: rel + "/", Typeflag: tar.TypeDir, Mode: 0700, ModTime: info.ModTime()})
		case info.Mode().IsRegular():
			if files++; files > MaxFolderFiles {
				return fmt.Errorf("the folder has more than %d files; share a smaller folder", MaxFolderFiles)
			}
			if total += info.Size(); total > MaxFolderBytes {
				return errors.New("the folder is larger than 2 GiB; share a smaller folder")
			}
			if err := tw.WriteHeader(&tar.Header{Name: rel, Typeflag: tar.TypeReg, Mode: 0600, Size: info.Size(), ModTime: info.ModTime()}); err != nil {
				return err
			}
			f, err := fsys.Open(p)
			if err != nil {
				return err
			}
			_, err = io.Copy(tw, io.LimitReader(f, info.Size()))
			f.Close()
			return err
		}
		return nil // links and devices are not shared
	})
	if err != nil {
		return err
	}
	return tw.Close()
}

// chunkReader reads a stored file's chunks in order, verifying each.
type chunkReader struct {
	s      *Store
	chunks []string
	buf    []byte
}

func (r *chunkReader) Read(p []byte) (int, error) {
	for len(r.buf) == 0 {
		if len(r.chunks) == 0 {
			return 0, io.EOF
		}
		b, err := r.s.Get(r.chunks[0])
		if err != nil {
			return 0, err
		}
		r.buf, r.chunks = b, r.chunks[1:]
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

// Extract unpacks a stored folder into dest, which must not exist yet. A
// package that fails part way leaves nothing behind.
func (s *Store) Extract(m Manifest, dest string) error {
	if m.Type != FolderType {
		return errors.New("not a folder")
	}
	if err := os.Mkdir(dest, 0700); err != nil {
		return err
	}
	if err := s.unpack(m, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	return nil
}

func (s *Store) unpack(m Manifest, dest string) error {
	tr := tar.NewReader(&chunkReader{s: s, chunks: append([]string(nil), m.Chunks...)})
	files := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if name == "." || path.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("the package names a path outside its folder: %q", h.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0700); err != nil {
				return err
			}
		case tar.TypeReg:
			if files++; files > MaxFolderFiles {
				return errors.New("the package holds too many files")
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, io.LimitReader(tr, h.Size))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("the package holds something other than files and folders: %q", h.Name)
		}
	}
}
