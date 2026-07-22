package production

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	testImage            = "example.invalid/jobcron:sha-0123456789ab"
	testDatabaseURL      = "postgres://user:pass@db.example.invalid:5432/jobcron?sslmode=require"
	testSessionSecret    = "synthetic-session-secret-at-least-32-bytes"
	testCredentialKey    = "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA="
	testDailyTime        = "06:15"
	testSignupAccessCode = "synthetic-cohort-code"
	testSponsorUserID    = "42"
	credentialKeyEnvName = "JOBCRON_CREDENTIAL_ENCRYPTION_KEY"
)

type composeConfig struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]any            `yaml:"volumes"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Command     []string          `yaml:"command"`
	Environment map[string]string `yaml:"environment"`
	Volumes     []composeVolume   `yaml:"volumes"`
}

type composeVolume struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
}

func TestProductionComposeRequiresCredentialEncryptionKey(t *testing.T) {
	cmd := composeCommand(false)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("docker compose config succeeded without JOBCRON_CREDENTIAL_ENCRYPTION_KEY")
	}
	if !strings.Contains(string(output), credentialKeyEnvName) {
		t.Fatalf("docker compose config failed without naming %s:\n%s", credentialKeyEnvName, output)
	}

	config := renderCompose(t)
	if got := config.Services["app"].Environment[credentialKeyEnvName]; got != testCredentialKey {
		t.Fatalf("%s = %q, want rendered synthetic key", credentialKeyEnvName, got)
	}
}

func TestProductionComposeHasNoJobcronConfigVolumeOrMount(t *testing.T) {
	config := renderCompose(t)
	app := config.Services["app"]
	for _, volume := range app.Volumes {
		if volume.Source == "jobcron_config" || volume.Target == "/root/.config/jobcron" {
			t.Fatalf("app retains filesystem credential storage mount: source=%q target=%q", volume.Source, volume.Target)
		}
	}
	if _, ok := config.Volumes["jobcron_config"]; ok {
		t.Fatal("top-level jobcron_config volume is still declared")
	}
}

func TestProductionComposeRetainsDatabaseSessionAndCaddyState(t *testing.T) {
	config := renderCompose(t)
	app := config.Services["app"]
	if got := app.Environment["DATABASE_URL"]; got != testDatabaseURL {
		t.Fatalf("DATABASE_URL = %q, want %q", got, testDatabaseURL)
	}
	if got := app.Environment["SESSION_SECRET"]; got != testSessionSecret {
		t.Fatalf("SESSION_SECRET = %q, want %q", got, testSessionSecret)
	}

	caddy := config.Services["caddy"]
	wantVolumes := map[string]string{"/data": "caddy_data", "/config": "caddy_config"}
	for _, volume := range caddy.Volumes {
		if wantSource, ok := wantVolumes[volume.Target]; ok {
			if volume.Source != wantSource {
				t.Errorf("caddy volume target %s uses source %q, want %q", volume.Target, volume.Source, wantSource)
			}
			delete(wantVolumes, volume.Target)
		}
	}
	for target := range wantVolumes {
		t.Errorf("caddy volume target %s is missing", target)
	}
	for _, name := range []string{"caddy_data", "caddy_config"} {
		if _, ok := config.Volumes[name]; !ok {
			t.Errorf("top-level volume %s is missing", name)
		}
	}
}

func TestProductionComposeUsesImmutableImageReference(t *testing.T) {
	config := renderCompose(t)
	immutableImage := regexp.MustCompile(`^(?:.+:sha-[0-9a-f]{12}|.+@sha256:[0-9a-f]{64})$`)
	if got := config.Services["app"].Image; !immutableImage.MatchString(got) {
		t.Fatalf("app image = %q, want sha-<12-hex> tag or sha256 digest", got)
	}
	if got := config.Services["app"].Image; got != testImage {
		t.Fatalf("app image = %q, want requested candidate %q", got, testImage)
	}
}

func TestProductionComposePreservesDailyTimeAndCommand(t *testing.T) {
	app := renderCompose(t).Services["app"]
	if got := app.Environment["JOBCRON_DAILY_SCRAPE_TIME"]; got != testDailyTime {
		t.Fatalf("JOBCRON_DAILY_SCRAPE_TIME = %q, want preserved %q", got, testDailyTime)
	}
	wantCommand := []string{"--no-open", "--host", "0.0.0.0", "--port", "7777"}
	if strings.Join(app.Command, "\x00") != strings.Join(wantCommand, "\x00") {
		t.Fatalf("app command = %q, want %q", app.Command, wantCommand)
	}
}

func TestProductionComposePassesCohortRuntimeVariables(t *testing.T) {
	app := renderCompose(t).Services["app"]
	if got := app.Environment["JOBCRON_SIGNUP_ACCESS_CODE"]; got != testSignupAccessCode {
		t.Fatalf("JOBCRON_SIGNUP_ACCESS_CODE = %q, want preserved synthetic code", got)
	}
	if got := app.Environment["JOBCRON_STAGE1_SPONSOR_USER_ID"]; got != testSponsorUserID {
		t.Fatalf("JOBCRON_STAGE1_SPONSOR_USER_ID = %q, want %q", got, testSponsorUserID)
	}
}

func TestProductionComposeLeavesCohortRuntimeVariablesUnset(t *testing.T) {
	output, err := composeCommand(true).CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config: %v\n%s", err, output)
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]*string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(output, &config); err != nil {
		t.Fatalf("parse rendered compose config: %v\n%s", err, output)
	}
	app := config.Services["app"]
	for _, name := range []string{"JOBCRON_SIGNUP_ACCESS_CODE", "JOBCRON_STAGE1_SPONSOR_USER_ID"} {
		value, ok := app.Environment[name]
		if !ok || value != nil {
			t.Errorf("%s = %v (present %v), want explicit null passthrough", name, value, ok)
		}
	}
}

func renderCompose(t *testing.T) composeConfig {
	t.Helper()
	cmd := composeCommand(true)
	cmd.Env = append(cmd.Env,
		"JOBCRON_SIGNUP_ACCESS_CODE="+testSignupAccessCode,
		"JOBCRON_STAGE1_SPONSOR_USER_ID="+testSponsorUserID,
	)
	return renderComposeCommand(t, cmd)
}

func renderComposeCommand(t *testing.T, cmd *exec.Cmd) composeConfig {
	t.Helper()
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config: %v\n%s", err, output)
	}

	var config composeConfig
	if err := yaml.Unmarshal(output, &config); err != nil {
		t.Fatalf("parse rendered compose config: %v\n%s", err, output)
	}
	return config
}

func composeCommand(includeCredentialKey bool) *exec.Cmd {
	cmd := exec.Command("docker", "compose", "-f", "compose.yaml", "config")
	cmd.Env = withoutEnvironment(os.Environ(),
		"JOBCRON_IMAGE",
		"DATABASE_URL",
		"SESSION_SECRET",
		credentialKeyEnvName,
		"JOBCRON_DAILY_SCRAPE_TIME",
		"JOBCRON_SIGNUP_ACCESS_CODE",
		"JOBCRON_STAGE1_SPONSOR_USER_ID",
	)
	cmd.Env = append(cmd.Env,
		"JOBCRON_IMAGE="+testImage,
		"DATABASE_URL="+testDatabaseURL,
		"SESSION_SECRET="+testSessionSecret,
		"JOBCRON_DAILY_SCRAPE_TIME="+testDailyTime,
	)
	if includeCredentialKey {
		cmd.Env = append(cmd.Env, credentialKeyEnvName+"="+testCredentialKey)
	}
	return cmd
}

func withoutEnvironment(environment []string, names ...string) []string {
	excluded := make(map[string]struct{}, len(names))
	for _, name := range names {
		excluded[name] = struct{}{}
	}

	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		name, _, _ := strings.Cut(entry, "=")
		if _, ok := excluded[name]; !ok {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
