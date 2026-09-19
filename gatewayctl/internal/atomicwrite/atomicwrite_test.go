package atomicwrite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCreatesFileWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	if err := Write(path, []byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestWriteOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	if err := Write(path, []byte("first")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := Write(path, []byte("second")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("got %q, want %q", got, "second")
	}
}

func TestWriteLeavesNoTempFilesBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.yaml")

	if err := Write(path, []byte("data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "out.yaml" {
		t.Errorf("expected only out.yaml in %s, found %v", dir, entries)
	}
}

func TestWriteFailsOnMissingParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist", "out.yaml")
	if err := Write(path, []byte("data")); err == nil {
		t.Error("expected an error writing into a non-existent directory, got nil")
	}
}
