//go:build !windows

package config

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type secretFileInfo struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (i secretFileInfo) Sys() any { return i.stat }

func TestSecretRejectsForeignOwnerAndHardlinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	stat := *info.Sys().(*syscall.Stat_t)
	stat.Uid++
	if secretOwned(secretFileInfo{info, &stat}) {
		t.Fatal("foreign owner accepted")
	}
	if err := os.Link(path, path+"-link"); err != nil {
		t.Fatal(err)
	}
	if _, err := Secret(map[string]string{"TEST_FILE": path}, "TEST"); err == nil {
		t.Fatal("hardlink accepted")
	}
}

func TestSecretRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Secret(map[string]string{"TEST_FILE": path}, "TEST"); err == nil {
		t.Fatal("FIFO accepted")
	}
}
