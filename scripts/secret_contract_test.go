package scripts

import "testing"

func TestProductionVerifierNeedsNoSecretValues(t *testing.T) {
	result := runProductionVerifier(t, nil, []string{"JOBCRON_IMAGE=" + syntheticImage, "JOBCRON_STAGE1_SPONSOR_USER_ID=42"})
	if result.err != nil {
		t.Fatalf("file-only contract failed: %v\n%s", result.err, result.output)
	}
}
