package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDocSnippetsInSync(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available; doc drift not checked")
	}
	root, err := filepath.Abs("..") // go/ -> repo root; go test cwd = package dir
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "scripts/doc-snippets-check.py")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("doc snippets out of sync:\n%s", out)
	}
}
