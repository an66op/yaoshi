package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadPasswordFileRequiresOwnerOnlyRegularFile(t *testing.T) {
	directory := t.TempDir()
	secure := filepath.Join(directory, "admin-password")
	if err := os.WriteFile(secure, []byte("V7!mQ2#zL9@pR4$x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := readPasswordFile(secure)
	if err != nil {
		t.Fatalf("readPasswordFile() error = %v", err)
	}
	if value != "V7!mQ2#zL9@pR4$x" {
		t.Fatalf("unexpected password contents %q", value)
	}

	insecure := filepath.Join(directory, "world-readable")
	if err := os.WriteFile(insecure, []byte("V7!mQ2#zL9@pR4$x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPasswordFile(insecure); err == nil {
		t.Fatal("expected world-readable password file to be rejected")
	}

	symlink := filepath.Join(directory, "password-link")
	if err := os.Symlink(secure, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := readPasswordFile(symlink); err == nil {
		t.Fatal("expected password symlink to be rejected")
	}
}
