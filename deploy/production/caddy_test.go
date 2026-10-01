package production

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Adapt only: no server, listener, certificate retrieval or application startup.
func TestCaddyAdaptsFileSecretWithoutPersistence(t *testing.T) {
	binary := os.Getenv("JOBCRON_CADDY_TEST_BINARY")
	if binary == "" {
		var err error
		binary, err = exec.LookPath("caddy")
		if err != nil {
			t.Skip("set JOBCRON_CADDY_TEST_BINARY to reviewed Caddy 2.8 binary for offline adapter check")
		}
	}
	dir := t.TempDir()
	input, err := os.ReadFile("Caddyfile")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Caddyfile")
	// Only fixture path substitution; syntax and directives come from the shipped file.
	text := strings.ReplaceAll(string(input), "/run/jobcron/caddy", dir)
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	const secret = "synthetic-proxy-secret"
	if err := os.WriteFile(filepath.Join(dir, "proxy-header"), []byte("header_up X-Jobcron-Proxy "+secret+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "adapt", "--config", path, "--adapter", "caddyfile")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "XDG_CONFIG_HOME=" + dir, "XDG_DATA_HOME=" + dir}
	output, err := cmd.Output()
	if err != nil {
		t.Fatal("Caddy adaptation failed (output withheld)")
	}
	var adapted struct {
		Admin struct {
			Disabled bool `json:"disabled"`
			Config   struct {
				Persist *bool `json:"persist"`
			} `json:"config"`
		} `json:"admin"`
	}
	if err := json.Unmarshal(output, &adapted); err != nil {
		t.Fatal("invalid adapted config")
	}
	if !adapted.Admin.Disabled || adapted.Admin.Config.Persist == nil || *adapted.Admin.Config.Persist {
		t.Fatal("Caddy persistence/admin remains enabled")
	}
	if !strings.Contains(string(output), secret) {
		t.Fatal("file secret missing from in-memory adapted config")
	}
}
