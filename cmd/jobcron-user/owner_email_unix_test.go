//go:build !windows

package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateOwnerEmailRejectsHardlinkCustody(t *testing.T) {
	path := ownerEmailFile(t, "private-owner@example.com", 0600)
	if err := os.Link(path, filepath.Join(t.TempDir(), "second-link")); err != nil {
		t.Fatal(err)
	}
	assertOwnerEmailCustodyRejected(t, path)
}

func TestCreateOwnerEmailRejectsForeignOwner(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("foreign ownership fixture requires root; do not elevate for this test")
	}
	path := ownerEmailFile(t, "private-owner@example.com", 0600)
	if err := os.Chown(path, 1, -1); err != nil {
		t.Fatal(err)
	}
	assertOwnerEmailCustodyRejected(t, path)
}

func assertOwnerEmailCustodyRejected(t *testing.T, path string) {
	t.Helper()
	var out bytes.Buffer
	in := strings.NewReader("unused-password\n")
	err := run(context.Background(), []string{"create-owner", "--database-url", "unused"}, envMap{
		"JOBCRON_OWNER_EMAIL_FILE": path,
	}, in, &out)
	if err == nil || err.Error() != "JOBCRON_OWNER_EMAIL_FILE: invalid secret input" || out.Len() != 0 || in.Len() != len("unused-password\n") {
		t.Fatal("unsafe owner email custody reached password/database work or disclosed input")
	}
}
