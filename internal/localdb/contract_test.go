package localdb

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

type composeContract struct {
	Services map[string]composeService `yaml:"services"`
	Volumes  map[string]composeVolume  `yaml:"volumes"`
}

type composeService struct {
	Image       string            `yaml:"image"`
	Environment map[string]string `yaml:"environment"`
	Ports       []string          `yaml:"ports"`
	Volumes     []string          `yaml:"volumes"`
	Healthcheck composeHealth     `yaml:"healthcheck"`
}

type composeHealth struct {
	Test     []string `yaml:"test"`
	Interval string   `yaml:"interval"`
	Retries  int      `yaml:"retries"`
}

type composeVolume struct {
	Name string `yaml:"name"`
}

func TestComposeContract(t *testing.T) {
	t.Parallel()

	deployPath := filepath.Join("..", "..", "deploy", "local", "compose.yaml")
	deployYAML, err := os.ReadFile(deployPath)
	if err != nil {
		t.Fatalf("read deploy definition: %v", err)
	}

	definitions := map[string][]byte{
		"embedded": ComposeYAML,
		"deploy":   deployYAML,
	}
	equal, err := decodedComposeEqual(ComposeYAML, deployYAML)
	if err != nil {
		t.Fatalf("compare decoded definitions: %v", err)
	}
	if !equal {
		t.Error("decoded embedded and deploy definitions differ")
	}

	for name, raw := range definitions {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			var got composeContract
			if err := yaml.Unmarshal(raw, &got); err != nil {
				t.Fatalf("decode %s definition: %v", name, err)
			}

			postgres, ok := got.Services["postgres"]
			if !ok {
				t.Error("service postgres is not defined")
				return
			}

			assertEqual(t, "image", postgres.Image, "postgres:18-alpine")
			assertEqual(t, "database", postgres.Environment["POSTGRES_DB"], "jobcron_dev")
			assertEqual(t, "ports", postgres.Ports, []string{"55432:5432"})
			assertEqual(t, "mounts", postgres.Volumes, []string{"jobcron-postgres18-cluster:/var/lib/postgresql"})
			assertEqual(t, "volume name", got.Volumes["jobcron-postgres18-cluster"].Name, "jobcron-postgres18-cluster")
			assertEqual(t, "health command", postgres.Healthcheck.Test, []string{"CMD-SHELL", "pg_isready -U postgres -d jobcron_dev"})
			interval, err := time.ParseDuration(postgres.Healthcheck.Interval)
			if err != nil {
				t.Errorf("healthcheck interval %q is invalid: %v", postgres.Healthcheck.Interval, err)
			} else {
				if interval > 5*time.Second {
					t.Errorf("healthcheck interval %s is not short (must be at most 5s)", interval)
				}
				if startupAllowance := interval * time.Duration(postgres.Healthcheck.Retries); startupAllowance < time.Minute {
					t.Errorf("healthcheck startup allowance = %s, want at least 1m", startupAllowance)
				}
			}
		})
	}
}

func TestDecodedComposeEqualityDetectsUnmodeledDrift(t *testing.T) {
	canonical := []byte("services:\n  postgres:\n    image: postgres:18-alpine\n")
	drifted := []byte("services:\n  postgres:\n    image: postgres:18-alpine\n    restart: unless-stopped\n")

	equal, err := decodedComposeEqual(canonical, drifted)
	if err != nil {
		t.Fatalf("compare decoded definitions: %v", err)
	}
	if equal {
		t.Fatal("definitions with an extra unmodeled field compared equal")
	}
}

func decodedComposeEqual(left, right []byte) (bool, error) {
	var leftDocument any
	if err := yaml.Unmarshal(left, &leftDocument); err != nil {
		return false, err
	}

	var rightDocument any
	if err := yaml.Unmarshal(right, &rightDocument); err != nil {
		return false, err
	}

	return reflect.DeepEqual(leftDocument, rightDocument), nil
}

func assertEqual(t *testing.T, field string, got, want any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", field, got, want)
	}
}
