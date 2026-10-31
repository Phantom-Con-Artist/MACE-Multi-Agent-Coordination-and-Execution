package blobs

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesRoundTripThroughChunks(t *testing.T) {
	s, _ := Open(t.TempDir())
	src := filepath.Join(t.TempDir(), "big.bin")
	data := bytes.Repeat([]byte("mace"), ChunkSize/2+123) // two chunks and a bit
	os.WriteFile(src, data, 0600)
	m, err := s.AddFile(src, "application/octet-stream")
	if err != nil || len(m.Chunks) != 3 || m.Size != int64(len(data)) {
		t.Fatalf("manifest %+v, %v", m, err)
	}
	if got, err := s.Manifest(m.ID()); err != nil || got.Size != m.Size {
		t.Fatalf("the manifest should be stored under its ID: %v", err)
	}
	// Identical chunks are stored once.
	if m.Chunks[0] != m.Chunks[1] {
		t.Fatal("repeated content should hash the same")
	}
	dest := filepath.Join(t.TempDir(), "out.bin")
	if err := s.Assemble(m, dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(dest); !bytes.Equal(b, data) {
		t.Fatal("the assembled file differs")
	}
}

func TestReceiverFetchesOnlyMissingVerifiedChunks(t *testing.T) {
	sender, _ := Open(t.TempDir())
	receiver, _ := Open(t.TempDir())
	src := filepath.Join(t.TempDir(), "f.txt")
	os.WriteFile(src, append(bytes.Repeat([]byte("a"), ChunkSize), 'b'), 0600)
	m, _ := sender.AddFile(src, "")
	first, _ := sender.Get(m.Chunks[0])
	receiver.Put(m.Chunks[0], first)
	if missing := receiver.Missing(m); len(missing) != 1 || missing[0] != m.Chunks[1] {
		t.Fatalf("only the second chunk is missing, got %v", missing)
	}
	if err := receiver.Put(m.Chunks[1], []byte("tampered")); err == nil {
		t.Fatal("a chunk that does not match its hash must be refused")
	}
	if err := receiver.Assemble(m, filepath.Join(t.TempDir(), "x")); err == nil {
		t.Fatal("assembling with a missing chunk must fail")
	}
}

func TestDamagedChunksAreNotServed(t *testing.T) {
	s, _ := Open(t.TempDir())
	h := Sum([]byte("hello"))
	s.Put(h, []byte("hello"))
	os.WriteFile(s.path(h), []byte("HELLO"), 0600)
	if _, err := s.Get(h); err == nil || s.Has(h) {
		t.Fatal("a damaged chunk should be refused and removed")
	}
}
