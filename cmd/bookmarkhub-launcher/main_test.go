package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadCurrentAcceptsUTF8BOM(t *testing.T) {
	root := t.TempDir()
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("{\"version\":\"0.1.0\"}\n")...)
	if err := os.WriteFile(filepath.Join(root, "current.json"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	current, err := readCurrent(root)
	if err != nil {
		t.Fatalf("read BOM-prefixed current.json: %v", err)
	}
	if current.Version != "0.1.0" {
		t.Fatalf("unexpected version %q", current.Version)
	}
}
