package scripts

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestReleaseConfigPackagesImporter(t *testing.T) {
	data, err := os.ReadFile("../.goreleaser.yml")
	if err != nil {
		t.Fatal(err)
	}

	var config struct {
		Builds []struct {
			ID      string   `yaml:"id"`
			Main    string   `yaml:"main"`
			Binary  string   `yaml:"binary"`
			Ldflags []string `yaml:"ldflags"`
		} `yaml:"builds"`
		Archives []struct {
			ID  string   `yaml:"id"`
			IDs []string `yaml:"ids"`
		} `yaml:"archives"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	builds := make(map[string]struct {
		main    string
		binary  string
		ldflags []string
	}, len(config.Builds))
	for _, build := range config.Builds {
		builds[build.ID] = struct {
			main    string
			binary  string
			ldflags []string
		}{build.Main, build.Binary, build.Ldflags}
	}

	app, ok := builds["jobcron"]
	if !ok || app.main != "./cmd/jobcron" || app.binary != "jobcron" {
		t.Fatalf("jobcron build = %#v, present = %v", app, ok)
	}
	if !containsVersionLdflag(app.ldflags) {
		t.Fatalf("jobcron ldflags must inject main.version: %q", app.ldflags)
	}

	importer, ok := builds["jobcron-import"]
	if !ok || importer.main != "./cmd/jobcron-import" || importer.binary != "jobcron-import" {
		t.Fatalf("jobcron-import build = %#v, present = %v", importer, ok)
	}
	if containsVersionLdflag(importer.ldflags) {
		t.Fatalf("jobcron-import must not inject main.version: %q", importer.ldflags)
	}

	if len(config.Archives) != 1 {
		t.Fatalf("archives = %d, want 1", len(config.Archives))
	}
	archiveIDs := strings.Join(config.Archives[0].IDs, ",")
	if archiveIDs != "jobcron,jobcron-import" {
		t.Fatalf("archive build IDs = %q, want jobcron,jobcron-import", archiveIDs)
	}
}

func containsVersionLdflag(flags []string) bool {
	for _, flag := range flags {
		if strings.Contains(flag, "main.version=") {
			return true
		}
	}
	return false
}
