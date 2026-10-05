package scripts

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func publicTestCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fixture CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestRuntimeVerifiedCAAndArchive(t *testing.T) {
	f := newRuntimeFixture(t)
	verified := runtimeSecretFields["DATABASE_URL"]
	secret := replaceRuntimeSecret(t, "DATABASE_URL", verified)
	for i := 0; i < 2; i++ {
		if result := f.run(t, secret, "prepare"); result.err != nil {
			t.Fatalf("prepare/reprepare: %v %s", result.err, result.output)
		}
	}
	caPath := filepath.Join(f.runDir, "rds-ca.pem")
	assertMode(t, caPath, 0600)
	if readFile(t, caPath) != readFile(t, filepath.Join(f.etcDir, "rds-ca.pem")) {
		t.Fatal("CA bytes changed")
	}
	if _, err := os.Stat(filepath.Join(f.runDir, "secrets", "secrets")); !os.IsNotExist(err) {
		t.Fatal("nested secrets collision")
	}
	// Use an explicit native executable path; the fake exercises selection,
	// not an invented real pg_dump version or dump/restore PASS.
	f.env = append(f.env, "JOBCRON_PG_DUMP="+filepath.Join(f.binDir, "pg_dump"))
	if result := f.run(t, secret, "archive"); result.err != nil {
		t.Fatalf("archive: %v %s", result.err, result.output)
	}
	log := readFile(t, f.logPath)
	if !strings.Contains(log, "sslmode=verify-full&sslrootcert=/run/jobcron/rds-ca.pem -Fc") || strings.Contains(log, "db%3Asecret%40value") {
		t.Fatal("archive lost verified URI or leaked password")
	}
	if result := f.run(t, secret, "cleanup"); result.err != nil {
		t.Fatal(result.err)
	}
	if _, err := os.Stat(caPath); !os.IsNotExist(err) {
		t.Fatal("runtime CA survived cleanup")
	}
	if _, err := os.Stat(filepath.Join(f.etcDir, "rds-ca.pem")); err != nil {
		t.Fatal("cleanup removed persistent public CA")
	}
}

func TestRuntimeCACustodyFailsClosed(t *testing.T) {
	for _, name := range []string{"missing", "symlink", "writable", "invalid PEM", "wrong owner"} {
		t.Run(name, func(t *testing.T) {
			f := newRuntimeFixture(t)
			path := filepath.Join(f.etcDir, "rds-ca.pem")
			switch name {
			case "missing":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				other := filepath.Join(f.etcDir, "other.pem")
				if err := os.Rename(path, other); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(other, path); err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.Chmod(path, 0666); err != nil {
					t.Fatal(err)
				}
			case "invalid PEM":
				writeFile(t, path, "invalid CA\n", 0600)
			case "wrong owner":
				writeExecutable(t, filepath.Join(f.binDir, "stat"), `#!/bin/sh
case "$*" in *rds-ca.pem*) if [ "$2" = %u ]; then printf 999999; else printf 600; fi;; *) if [ "$2" = %u ]; then id -u; else printf 700; fi;; esac
`)
			}
			result := f.run(t, validRuntimeSecret(), "prepare")
			if result.err == nil {
				t.Fatal("unsafe CA accepted")
			}
			assertNoRuntimeSecret(t, result.output)
			if strings.Contains(readOptionalFile(t, f.logPath), "aws ") {
				t.Fatal("retrieval before CA validation")
			}
		})
	}
}

func TestRuntimeArchiveRejectsCommandInjection(t *testing.T) {
	for _, command := range []string{"pg_dump --version", "relative/pg_dump", "/missing/pg_dump"} {
		f := newRuntimeFixture(t)
		if result := f.run(t, validRuntimeSecret(), "prepare"); result.err != nil {
			t.Fatal(result.err)
		}
		f.env = append(f.env, "JOBCRON_PG_DUMP="+command)
		if result := f.run(t, validRuntimeSecret(), "archive"); result.err == nil {
			t.Fatal("invalid executable selection accepted")
		}
		if strings.Contains(readFile(t, f.logPath), "pg_dump ") {
			t.Fatal("invalid selection reached pg_dump")
		}
	}
}

func TestRuntimePreparePreservesUnexpectedSecretEvidence(t *testing.T) {
	f := newRuntimeFixture(t)
	path := filepath.Join(f.runDir, "secrets", "unexpected-evidence")
	writeFile(t, path, "preserve\n", 0600)
	if result := f.run(t, validRuntimeSecret(), "prepare"); result.err == nil {
		t.Fatal("prepare allowed a nested secrets collision")
	}
	if readFile(t, path) != "preserve\n" {
		t.Fatal("recovery evidence changed")
	}
	if _, err := os.Stat(filepath.Join(f.runDir, "secrets", "secrets")); !os.IsNotExist(err) {
		t.Fatal("prepare nested the secrets directory")
	}
}

func TestRDSRoleCAAndAmbientSelectorsFailClosed(t *testing.T) {
	for _, name := range []string{"missing CA", "writable CA", "invalid CA", "ambient selector"} {
		t.Run(name, func(t *testing.T) {
			f := newPrivateOpsFixture(t)
			var caPath string
			for _, entry := range f.env {
				if strings.HasPrefix(entry, "JOBCRON_RDS_CA_FILE=") {
					caPath = strings.TrimPrefix(entry, "JOBCRON_RDS_CA_FILE=")
				}
			}
			switch name {
			case "missing CA":
				if err := os.Remove(caPath); err != nil {
					t.Fatal(err)
				}
			case "writable CA":
				if err := os.Chmod(caPath, 0666); err != nil {
					t.Fatal(err)
				}
			case "invalid CA":
				writeFile(t, caPath, "invalid\n", 0600)
			case "ambient selector":
				f.env = append(f.env, "PGHOSTADDR=unapproved.invalid")
			}
			result := f.run(t, rdsRoleHelper, "master-private\napp-private\n")
			if result.err == nil {
				t.Fatal("unsafe role connection accepted")
			}
			log := readOptionalFile(t, f.commandLog)
			if strings.Contains(log, "psql ") || strings.Contains(log+result.output, "master-private") || strings.Contains(log+result.output, "app-private") {
				t.Fatal("unsafe input reached mutation or output")
			}
		})
	}
}
