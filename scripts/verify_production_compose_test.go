package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	productionVerifier = "scripts/verify-production-compose.sh"
	syntheticImage     = "example.invalid/jobcron:sha-0123456789ab"
	syntheticDatabase  = "postgres://synthetic:synthetic@db.example.invalid/jobcron?sslmode=require"
	syntheticSession   = "synthetic-session-secret-at-least-32-bytes"
	syntheticKey       = "MDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDAwMDA="
)

var syntheticProductionEnvironment = []string{
	"JOBCRON_IMAGE=" + syntheticImage,
	"DATABASE_URL=" + syntheticDatabase,
	"SESSION_SECRET=" + syntheticSession,
	"JOBCRON_CREDENTIAL_ENCRYPTION_KEY=" + syntheticKey,
}

func TestProductionComposeVerifierSyntax(t *testing.T) {
	cmd := exec.Command("sh", "-n", productionVerifier)
	cmd.Dir = filepath.Clean(filepath.Join(".."))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sh -n %s: %v\n%s", productionVerifier, err, output)
	}
}

func TestProductionComposeVerifierAcceptsImmutableReferences(t *testing.T) {
	tests := []struct {
		name  string
		image string
	}{
		{name: "commit tag", image: syntheticImage},
		{name: "sha256 digest", image: "example.invalid/jobcron@sha256:" + strings.Repeat("a", 64)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runProductionVerifier(t, nil, replaceEnvironment(syntheticProductionEnvironment, "JOBCRON_IMAGE", test.image))
			if result.err != nil {
				t.Fatalf("verifier rejected immutable image reference: %v\n%s", result.err, result.output)
			}
			if !strings.Contains(result.output, "production Compose contract verified") {
				t.Fatalf("success output = %q, want contract confirmation", result.output)
			}
		})
	}
}

func TestProductionComposeVerifierRequiresSyntheticInputs(t *testing.T) {
	for _, name := range []string{
		"JOBCRON_IMAGE",
		"DATABASE_URL",
		"SESSION_SECRET",
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY",
	} {
		t.Run(name, func(t *testing.T) {
			result := runProductionVerifier(t, nil, removeEnvironment(syntheticProductionEnvironment, name))
			assertRejectedContract(t, result, name)
		})
	}
}

func TestProductionComposeVerifierDoesNotDiscloseInvalidTemporaryDirectory(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join(".."))
	marker := "must-not-leak-tmpdir-marker"
	invalidTemporaryDirectory := filepath.Join(t.TempDir(), marker, "missing")
	cmd := exec.Command("sh", productionVerifier)
	cmd.Dir = repoRoot
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + invalidTemporaryDirectory,
	}, syntheticProductionEnvironment...)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("verifier accepted invalid TMPDIR %q", invalidTemporaryDirectory)
	}
	want := "production Compose contract failed: private rendered Compose file"
	if got := strings.TrimSpace(string(output)); got != want {
		t.Fatalf("invalid TMPDIR output = %q, want only %q", got, want)
	}
	if strings.Contains(string(output), marker) || strings.Contains(string(output), invalidTemporaryDirectory) {
		t.Fatalf("invalid TMPDIR output disclosed environment-controlled path: %q", output)
	}
}

func TestProductionComposeVerifierRejectsUnsafeTopology(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		mutate   func(string) string
	}{
		{
			name:     "app filesystem mount",
			contract: "services.app.volumes",
			mutate: replaceOnce(
				"    expose:\n      - \"7777\"",
				"    volumes:\n      - ./state:/root/.config/jobcron\n    expose:\n      - \"7777\"",
			),
		},
		{
			name:     "app publishes container port directly",
			contract: "services.app.ports",
			mutate: replaceOnce(
				"    expose:\n      - \"7777\"",
				"    ports:\n      - \"8888:7777\"",
			),
		},
		{
			name:     "app publishes Caddy HTTP port",
			contract: "services.app.ports",
			mutate: replaceOnce(
				"    expose:\n      - \"7777\"",
				"    expose:\n      - \"7777\"\n    ports:\n      - \"80:7777\"",
			),
		},
		{
			name:     "Caddy omits HTTP port",
			contract: "services.caddy.ports",
			mutate:   replaceOnce("      - \"80:80\"\n", ""),
		},
		{
			name:     "Caddy omits HTTPS port",
			contract: "services.caddy.ports",
			mutate:   replaceOnce("      - \"443:443\"\n", ""),
		},
		{
			name:     "Caddy publishes an extra port",
			contract: "services.caddy.ports",
			mutate: replaceOnce(
				"      - \"443:443\"",
				"      - \"443:443\"\n      - \"8080:8080\"",
			),
		},
		{
			name:     "legacy app volume remains declared",
			contract: "volumes.jobcron_config",
			mutate: replaceOnce(
				"volumes:\n  caddy_data:\n  caddy_config:",
				"volumes:\n  caddy_data:\n  caddy_config:\n  jobcron_config:",
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runProductionVerifier(t, test.mutate, syntheticProductionEnvironment)
			assertRejectedContract(t, result, test.contract)
		})
	}
}

func TestProductionComposeVerifierRejectsMissingAppEnvironment(t *testing.T) {
	for _, name := range []string{
		"DATABASE_URL",
		"SESSION_SECRET",
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY",
		"JOBCRON_ENV",
		"JOBCRON_HOST",
		"JOBCRON_PORT",
		"JOBCRON_NO_OPEN",
		"JOBCRON_SCHEDULER_ENABLED",
		"JOBCRON_DAILY_SCRAPE_TIME",
	} {
		t.Run(name, func(t *testing.T) {
			result := runProductionVerifier(t, removeComposeEnvironment(name), syntheticProductionEnvironment)
			assertRejectedContract(t, result, "services.app.environment."+name)
		})
	}
}

func TestProductionComposeVerifierRejectsMismatchedSensitiveEnvironment(t *testing.T) {
	tests := []struct {
		name          string
		mismatchValue string
		mutate        func(string) string
	}{
		{
			name:          "DATABASE_URL",
			mismatchValue: "postgres://mismatch.invalid/jobcron",
			mutate: replaceOnce(
				`      DATABASE_URL: "${DATABASE_URL:?set DATABASE_URL in .env}"`,
				`      DATABASE_URL: "postgres://mismatch.invalid/jobcron"`,
			),
		},
		{
			name:          "SESSION_SECRET",
			mismatchValue: "mismatched-session-value",
			mutate: replaceOnce(
				`      SESSION_SECRET: "${SESSION_SECRET:?set SESSION_SECRET in .env}"`,
				`      SESSION_SECRET: "mismatched-session-value"`,
			),
		},
		{
			name:          "JOBCRON_CREDENTIAL_ENCRYPTION_KEY",
			mismatchValue: "mismatched-credential-key",
			mutate: replaceOnce(
				"      JOBCRON_CREDENTIAL_ENCRYPTION_KEY: >-\n        ${JOBCRON_CREDENTIAL_ENCRYPTION_KEY:?set JOBCRON_CREDENTIAL_ENCRYPTION_KEY in .env}",
				`      JOBCRON_CREDENTIAL_ENCRYPTION_KEY: "mismatched-credential-key"`,
			),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runProductionVerifier(t, test.mutate, syntheticProductionEnvironment)
			assertRejectedContract(t, result, "services.app.environment."+test.name)
			if strings.Contains(result.output, test.mismatchValue) {
				t.Fatalf("rejection output disclosed mismatched %s value: %q", test.name, result.output)
			}
		})
	}
}

func TestProductionComposeVerifierRejectsWrongProductionSettings(t *testing.T) {
	tests := []struct {
		name     string
		contract string
		mutate   func(string) string
	}{
		{
			name:     "non-production mode",
			contract: "services.app.environment.JOBCRON_ENV",
			mutate:   replaceOnce("      JOBCRON_ENV: production", "      JOBCRON_ENV: development"),
		},
		{
			name:     "non-public app bind address",
			contract: "services.app.environment.JOBCRON_HOST",
			mutate:   replaceOnce("      JOBCRON_HOST: 0.0.0.0", "      JOBCRON_HOST: 127.0.0.1"),
		},
		{
			name:     "unexpected app port",
			contract: "services.app.environment.JOBCRON_PORT",
			mutate:   replaceOnce("      JOBCRON_PORT: \"7777\"", "      JOBCRON_PORT: \"8888\""),
		},
		{
			name:     "browser opening enabled",
			contract: "services.app.environment.JOBCRON_NO_OPEN",
			mutate:   replaceOnce("      JOBCRON_NO_OPEN: \"1\"", "      JOBCRON_NO_OPEN: \"0\""),
		},
		{
			name:     "disabled scheduler",
			contract: "services.app.environment.JOBCRON_SCHEDULER_ENABLED",
			mutate:   replaceOnce("      JOBCRON_SCHEDULER_ENABLED: \"1\"", "      JOBCRON_SCHEDULER_ENABLED: \"0\""),
		},
		{
			name:     "unexpected scrape time",
			contract: "services.app.environment.JOBCRON_DAILY_SCRAPE_TIME",
			mutate:   replaceOnce("      JOBCRON_DAILY_SCRAPE_TIME: \"05:00\"", "      JOBCRON_DAILY_SCRAPE_TIME: \"06:00\""),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runProductionVerifier(t, test.mutate, syntheticProductionEnvironment)
			assertRejectedContract(t, result, test.contract)
		})
	}
}

func TestProductionComposeVerifierRejectsForbiddenAppEnvironment(t *testing.T) {
	for _, name := range []string{
		"JOBCRON_DEMO",
		"JOBCRON_ADMIN_TOKEN",
		"JOBCRON_WORKNET_KEY",
		"JOBCRON_PROXY_SECRET",
	} {
		t.Run(name, func(t *testing.T) {
			forbiddenValue := "forbidden-synthetic-value-" + name
			result := runProductionVerifier(t, replaceOnce(
				"      JOBCRON_NO_OPEN: \"1\"",
				"      JOBCRON_NO_OPEN: \"1\"\n      "+name+": \""+forbiddenValue+"\"",
			), syntheticProductionEnvironment)
			assertRejectedContract(t, result, "services.app.environment."+name)
			if strings.Contains(result.output, forbiddenValue) {
				t.Fatalf("rejection output disclosed forbidden %s value: %q", name, result.output)
			}
		})
	}
}

func TestProductionComposeVerifierRejectsMutableOrMalformedImage(t *testing.T) {
	for _, image := range []string{
		"example.invalid/jobcron:latest",
		"example.invalid/jobcron:sha-0123456789a",
		"example.invalid/jobcron:sha-0123456789ag",
		"example.invalid/jobcron@sha256:" + strings.Repeat("a", 63),
	} {
		result := runProductionVerifier(t, nil, replaceEnvironment(syntheticProductionEnvironment, "JOBCRON_IMAGE", image))
		assertRejectedContract(t, result, "services.app.image")
	}
}

func TestProductionComposeCIUsesSyntheticInputs(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatalf("read CI workflow: %v", err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string            `yaml:"name"`
				Run  string            `yaml:"run"`
				Env  map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatalf("parse CI workflow: %v", err)
	}

	var contractStep *struct {
		Name string            `yaml:"name"`
		Run  string            `yaml:"run"`
		Env  map[string]string `yaml:"env"`
	}
	for index := range workflow.Jobs["test"].Steps {
		step := &workflow.Jobs["test"].Steps[index]
		if step.Name == "production Compose contract" {
			contractStep = step
			break
		}
	}
	if contractStep == nil {
		t.Fatal("CI test job is missing the production Compose contract step")
	}
	for _, command := range []string{
		"sh -n scripts/verify-production-compose.sh",
		"go test ./scripts -run ProductionCompose -count=1",
		"sh scripts/verify-production-compose.sh",
	} {
		if !strings.Contains(contractStep.Run, command) {
			t.Errorf("production Compose CI step is missing %q", command)
		}
	}

	wantEnvironment := map[string]string{
		"JOBCRON_IMAGE":                     syntheticImage,
		"DATABASE_URL":                      syntheticDatabase,
		"SESSION_SECRET":                    syntheticSession,
		"JOBCRON_CREDENTIAL_ENCRYPTION_KEY": syntheticKey,
	}
	if len(contractStep.Env) != len(wantEnvironment) {
		t.Fatalf("production Compose CI environment has %d entries, want %d synthetic inputs", len(contractStep.Env), len(wantEnvironment))
	}
	for name, want := range wantEnvironment {
		if got := contractStep.Env[name]; got != want {
			t.Errorf("production Compose CI %s = %q, want documented synthetic placeholder", name, got)
		}
	}
}

type verifierResult struct {
	output string
	err    error
}

func runProductionVerifier(t *testing.T, mutate func(string) string, environment []string) verifierResult {
	t.Helper()
	repoRoot := filepath.Clean(filepath.Join(".."))
	root := t.TempDir()
	for _, directory := range []string{"scripts", filepath.Join("deploy", "production"), "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatalf("create fixture directory %s: %v", directory, err)
		}
	}

	copyFixtureFile(t, filepath.Join(repoRoot, productionVerifier), filepath.Join(root, productionVerifier), nil)
	copyFixtureFile(t,
		filepath.Join(repoRoot, "deploy", "production", "compose.yaml"),
		filepath.Join(root, "deploy", "production", "compose.yaml"),
		mutate,
	)
	copyFixtureFile(t,
		filepath.Join(repoRoot, "deploy", "production", "Caddyfile"),
		filepath.Join(root, "deploy", "production", "Caddyfile"),
		nil,
	)

	cmd := exec.Command("sh", productionVerifier)
	cmd.Dir = root
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"TMPDIR=" + filepath.Join(root, "tmp"),
	}, environment...)
	output, err := cmd.CombinedOutput()
	assertNoRenderedComposeRemains(t, filepath.Join(root, "tmp"))
	return verifierResult{output: string(output), err: err}
}

func copyFixtureFile(t *testing.T, source, destination string, mutate func(string) string) {
	t.Helper()
	contents, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read fixture %s: %v", source, err)
	}
	text := string(contents)
	if mutate != nil {
		text = mutate(text)
	}
	if err := os.WriteFile(destination, []byte(text), 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", destination, err)
	}
}

func assertRejectedContract(t *testing.T, result verifierResult, contract string) {
	t.Helper()
	if result.err == nil {
		t.Fatalf("verifier accepted broken %s contract:\n%s", contract, result.output)
	}
	if !strings.Contains(result.output, contract) {
		t.Fatalf("rejection output = %q, want contract field %s", result.output, contract)
	}
	for _, value := range []string{syntheticImage, syntheticDatabase, syntheticSession, syntheticKey} {
		if strings.Contains(result.output, value) {
			t.Fatalf("rejection output disclosed a synthetic environment value: %q", result.output)
		}
	}
}

func assertNoRenderedComposeRemains(t *testing.T, temporaryDirectory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(temporaryDirectory, "jobcron-production-compose.*"))
	if err != nil {
		t.Fatalf("glob rendered Compose files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("rendered Compose file was not removed: %v", matches)
	}
}

func replaceOnce(old, new string) func(string) string {
	return func(contents string) string {
		if strings.Count(contents, old) != 1 {
			panic("fixture marker must occur exactly once: " + old)
		}
		return strings.Replace(contents, old, new, 1)
	}
}

func removeComposeEnvironment(name string) func(string) string {
	return func(contents string) string {
		lines := strings.Split(contents, "\n")
		result := make([]string, 0, len(lines))
		removed := false
		for index := 0; index < len(lines); index++ {
			if strings.HasPrefix(lines[index], "      "+name+":") {
				removed = true
				if strings.HasSuffix(lines[index], ">-") && index+1 < len(lines) {
					index++
				}
				continue
			}
			result = append(result, lines[index])
		}
		if !removed {
			panic("environment fixture not found: " + name)
		}
		return strings.Join(result, "\n")
	}
}

func removeEnvironment(environment []string, name string) []string {
	result := make([]string, 0, len(environment))
	for _, entry := range environment {
		if !strings.HasPrefix(entry, name+"=") {
			result = append(result, entry)
		}
	}
	return result
}

func replaceEnvironment(environment []string, name, value string) []string {
	return append(removeEnvironment(environment, name), name+"="+value)
}
