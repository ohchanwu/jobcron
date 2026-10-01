package config

import "testing"

func TestProductionRejectsSecretArgument(t *testing.T) {
	_, err := Load([]string{"--worknet-api-key", "synthetic-key"}, map[string]string{"JOBCRON_ENV": "production"})
	if err == nil || err.Error() != "production requires JOBCRON_WORKNET_KEY_FILE, not --worknet-api-key" {
		t.Fatalf("argument accepted: %v", err)
	}
}
