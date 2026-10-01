package config

import (
	"errors"
	"os"
)

// Unix ownership/mode guarantees cannot be established by portable Windows IO.
// Windows development retains the non-production direct environment contract.
func openSecret(string) (*os.File, error) {
	return nil, errors.New("secret files require Unix permissions")
}
func secretOwned(os.FileInfo) bool { return false }
