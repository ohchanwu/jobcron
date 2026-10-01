package main

import (
	"io"
	"strings"
	"testing"
)

func TestProductionPasswordsRejectDirectEnvironment(t *testing.T) {
	for _, name := range []string{"JOBCRON_DATABASE_PASSWORD", "JOBCRON_OWNER_PASSWORD", "JOBCRON_USER_PASSWORD"} {
		_, err := commandPassword(envMap{"JOBCRON_ENV": "production", name: "synthetic-password"}, name, "test", strings.NewReader(""), io.Discard)
		if err == nil || err.Error() != name+": production requires file input" {
			t.Fatalf("direct production password accepted: %v", err)
		}
	}
}
