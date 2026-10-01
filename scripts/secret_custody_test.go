package scripts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeRejectsPersistentNestedDirectory(t *testing.T) {
	f := newRuntimeFixture(t)
	if err := os.Mkdir(filepath.Join(f.runDir, "secrets"), 0700); err != nil {
		t.Fatal(err)
	}
	writeExecutable(t, filepath.Join(f.binDir, "findmnt"), "#!/bin/sh\ncase \"$*\" in */secrets) printf ext4;; *) printf tmpfs;; esac\n")
	r := f.run(t, validRuntimeSecret(), "prepare")
	if r.err == nil {
		t.Fatal("persistent nested secret directory accepted")
	}
	if strings.Contains(readOptionalFile(t, f.logPath), "aws ") {
		t.Fatal("retrieved before custody check")
	}
}
