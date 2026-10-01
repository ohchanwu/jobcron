package scripts

import "testing"

func TestProductionVerifierRejectsUncontractedEnvironment(t *testing.T) {
	for _, marker := range []string{"      JOBCRON_NO_OPEN: \"1\"", "      AWS_EC2_METADATA_DISABLED: \"true\"\n    volumes:\n      - ./Caddyfile"} {
		result := runProductionVerifier(t, replaceOnce(marker, "      UNREVIEWED_SECRET: synthetic-unexpected\n"+marker), syntheticProductionEnvironment)
		if result.err == nil {
			t.Fatal("uncontracted environment accepted")
		}
	}
}
