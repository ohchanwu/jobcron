package production

import "testing"

func TestProductionComposeMountsRDSCAReadOnly(t *testing.T) {
	app := renderCompose(t).Services["app"]
	for _, volume := range app.Volumes {
		if volume.Source == "/run/jobcron/rds-ca.pem" {
			if volume.Type != "bind" || volume.Target != volume.Source || !volume.ReadOnly || volume.Bind.CreateHostPath {
				t.Fatal("RDS CA mount must be read-only and fail if the file is missing")
			}
			return
		}
	}
	t.Fatal("RDS CA mount missing")
}
