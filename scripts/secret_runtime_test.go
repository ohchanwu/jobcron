package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeRejectsPersistentSecretStorage(t *testing.T) {
	f := newRuntimeFixture(t)
	writeExecutable(t, filepath.Join(f.binDir, "findmnt"), "#!/bin/sh\nprintf ext4\n")
	r := f.run(t, validRuntimeSecret(), "prepare")
	if r.err == nil {
		t.Fatal("persistent runtime filesystem accepted")
	}
	if strings.Contains(readOptionalFile(t, f.logPath), "aws ") {
		t.Fatal("retrieved secrets before tmpfs check")
	}
}

func TestRuntimeRejectsSymlinkDirectory(t *testing.T) {
	f := newRuntimeFixture(t)
	outside := filepath.Join(f.root, "outside")
	if err := os.Mkdir(outside, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.runDir, "caddy")); err != nil {
		t.Fatal(err)
	}
	r := f.run(t, validRuntimeSecret(), "prepare")
	if r.err == nil {
		t.Fatal("symlink runtime accepted")
	}
	if strings.Contains(readOptionalFile(t, f.logPath), "aws ") {
		t.Fatal("retrieved secrets before custody check")
	}
}

func TestRuntimeRejectsSwap(t *testing.T) {
	f := newRuntimeFixture(t)
	writeExecutable(t, filepath.Join(f.binDir, "swapon"), "#!/bin/sh\nprintf /synthetic/swap\n")
	r := f.run(t, validRuntimeSecret(), "prepare")
	if r.err == nil {
		t.Fatal("swappable secret storage accepted")
	}
	if strings.Contains(readOptionalFile(t, f.logPath), "aws ") {
		t.Fatal("retrieved secrets before swap check")
	}
}
