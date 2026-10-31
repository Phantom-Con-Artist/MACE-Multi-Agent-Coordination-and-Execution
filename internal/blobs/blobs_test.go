package blobs

import (
	"archive/tar"
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

func TestFoldersPackAndUnpackSafely(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "site", "css"), 0700)
	os.MkdirAll(filepath.Join(src, ".git"), 0700)
	os.MkdirAll(filepath.Join(src, "node_modules", "x"), 0700)
	os.WriteFile(filepath.Join(src, "site", "index.html"), []byte("<h1>hi</h1>"), 0600)
	os.WriteFile(filepath.Join(src, "site", "css", "a.css"), []byte("h1{}"), 0600)
	os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref"), 0600)
	os.WriteFile(filepath.Join(src, "node_modules", "x", "i.js"), []byte("x"), 0600)
	os.Symlink("/etc/passwd", filepath.Join(src, "site", "link"))
	s, _ := Open(t.TempDir())
	m, err := s.AddFolder(os.DirFS(src), ".", "project")
	if err != nil || m.Type != FolderType {
		t.Fatalf("%+v %v", m, err)
	}
	dest := filepath.Join(t.TempDir(), "out")
	if err := s.Extract(m, dest); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "site", "css", "a.css")); string(b) != "h1{}" {
		t.Fatal("nested files should arrive")
	}
	for _, gone := range []string{".git", "node_modules", "site/link"} {
		if _, err := os.Lstat(filepath.Join(dest, gone)); err == nil {
			t.Fatalf("%s should not be shared", gone)
		}
	}
}

func TestPackagesCannotWriteOutsideTheirFolder(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	tw.WriteHeader(&tar.Header{Name: "../escape.txt", Typeflag: tar.TypeReg, Mode: 0600, Size: 2})
	tw.Write([]byte("no"))
	tw.Close()
	s, _ := Open(t.TempDir())
	m, _ := s.Add(&buf, "evil", FolderType)
	root := t.TempDir()
	if err := s.Extract(m, filepath.Join(root, "in")); err == nil {
		t.Fatal("a path that leaves the folder must be refused")
	}
	if _, err := os.Stat(filepath.Join(root, "escape.txt")); err == nil {
		t.Fatal("nothing may be written outside the folder")
	}
}
