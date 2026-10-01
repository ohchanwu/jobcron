package config

import "testing"

func TestProductionRejectsDirectSecrets(t *testing.T) {
	for _, name := range []string{"DATABASE_URL", "SESSION_SECRET", "JOBCRON_CREDENTIAL_ENCRYPTION_KEY", "JOBCRON_SIGNUP_ACCESS_CODE", "JOBCRON_PROXY_SECRET", "JOBCRON_ADMIN_TOKEN", "JOBCRON_WORKNET_KEY"} {
		_, err := Load(nil, map[string]string{"JOBCRON_ENV": "production", name: "synthetic-value"})
		if err == nil || err.Error() != name+": production requires file input" {
			t.Errorf("direct %s not rejected at boundary: %v", name, err)
		}
	}
}
