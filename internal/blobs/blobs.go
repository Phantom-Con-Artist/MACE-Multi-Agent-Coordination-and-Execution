// Package blobs stores files as content-addressed chunks.
//
// A file is split into chunks of up to ChunkSize bytes, each stored under the
// SHA-256 of its content. A manifest lists the chunk hashes in order with the
// file's name, size, and type; the file's ID is the SHA-256 of its manifest.
// The same content is stored once however often it is shared, a transfer
// fetches only the chunks the receiver lacks, and every chunk is verified
// against its hash when it arrives (docs/DESIGN-HUB-WORKER.md, Files).
package blobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// ChunkSize is the largest chunk. Four MiB keeps a lost connection cheap to
// resume and a chunk small enough to verify before it is stored.
const ChunkSize = 4 << 20

// Manifest describes one file.
type Manifest struct {
	Version int      `json:"v"`
	Name    string   `json:"name"`
	Size    int64    `json:"size"`
	Type    string   `json:"type,omitempty"`
	Chunks  []string `json:"chunks"`
}

// ID is the hash of the manifest's canonical encoding.
func (m Manifest) ID() string {
	b, _ := json.Marshal(m)
	return Sum(b)
}

// Sum is the hex SHA-256 used for every address.
func Sum(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

var hashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidHash reports whether s has the form of an address.
func ValidHash(s string) bool { return hashPattern.MatchString(s) }

// Store keeps chunks and manifests under one folder.
type Store struct{ dir string }

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

func (s *Store) path(hash string) string {
	return filepath.Join(s.dir, hash[:2], hash)
}

// Has reports whether a chunk or manifest is stored.
func (s *Store) Has(hash string) bool {
	if !ValidHash(hash) {
		return false
	}
	_, err := os.Stat(s.path(hash))
	return err == nil
}

// Put stores content under its hash after checking that it matches.
func (s *Store) Put(hash string, data []byte) error {
	if !ValidHash(hash) || Sum(data) != hash {
		return fmt.Errorf("content does not match %s", short(hash))
	}
	if s.Has(hash) {
		return nil
	}
	p := s.path(hash)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Get reads stored content and checks it against its hash, so a damaged
// chunk is never served.
func (s *Store) Get(hash string) ([]byte, error) {
	if !ValidHash(hash) {
		return nil, errors.New("invalid address")
	}
	b, err := os.ReadFile(s.path(hash))
	if err != nil {
		return nil, err
	}
	if Sum(b) != hash {
		os.Remove(s.path(hash))
		return nil, fmt.Errorf("stored content for %s was damaged and removed", short(hash))
	}
	return b, nil
}

// AddFile splits a file into chunks, stores them and its manifest, and
// returns the manifest.
func (s *Store) AddFile(path, kind string) (Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	return s.Add(f, filepath.Base(path), kind)
}

// Add stores everything read from f as a file called name.
func (s *Store) Add(f io.Reader, name, kind string) (Manifest, error) {
	m := Manifest{Version: 1, Name: name, Type: kind, Chunks: []string{}}
	buf := make([]byte, ChunkSize)
	for {
		n, err := io.ReadFull(f, buf)
		if n > 0 {
			hash := Sum(buf[:n])
			if err := s.Put(hash, buf[:n]); err != nil {
				return Manifest{}, err
			}
			m.Chunks = append(m.Chunks, hash)
			m.Size += int64(n)
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return Manifest{}, err
		}
	}
	b, _ := json.Marshal(m)
	return m, s.Put(Sum(b), b)
}

// Manifest loads a stored manifest by its ID.
func (s *Store) Manifest(id string) (Manifest, error) {
	b, err := s.Get(id)
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil || m.Version != 1 {
		return Manifest{}, errors.New("not a file manifest")
	}
	return m, nil
}

// Missing lists the chunks of a manifest that are not stored here.
func (s *Store) Missing(m Manifest) []string {
	out := []string{}
	for _, c := range m.Chunks {
		if !s.Has(c) && !contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// Assemble writes a complete file to dest, refusing if a chunk is missing
// or the size differs from the manifest.
func (s *Store) Assemble(m Manifest, dest string) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".mace-part-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	var size int64
	for _, c := range m.Chunks {
		b, err := s.Get(c)
		if err != nil {
			tmp.Close()
			return err
		}
		if _, err := tmp.Write(b); err != nil {
			tmp.Close()
			return err
		}
		size += int64(len(b))
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if size != m.Size {
		return fmt.Errorf("assembled %d bytes, the manifest says %d", size, m.Size)
	}
	if err := os.Chmod(tmp.Name(), 0600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}
