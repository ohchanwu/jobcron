//go:build !windows

package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// The public CA may be readable, but only root or this operator may replace it.
func checkCAFile(path string) error {
	invalid := errors.New("user: invalid CA file custody")
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return invalid
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return invalid
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || (stat.Uid != 0 && stat.Uid != uint32(os.Geteuid())) {
			return invalid
		}
		// A root-owned sticky temporary ancestor cannot be used by another
		// user to replace the next owned directory (e.g. Linux /tmp).
		if info.Mode().Perm()&022 != 0 && !(info.IsDir() && info.Mode()&os.ModeSticky != 0 && stat.Uid == 0) {
			return invalid
		}
		if current == path && (!info.Mode().IsRegular() || info.Size() == 0) {
			return invalid
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
