//go:build !windows

package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testRDSHost = "jobcron.abc123.ap-northeast-2.rds.amazonaws.com"

func tunnelCertificate(t *testing.T, host string) ([]byte, tls.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, DNSNames: []string{host}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func tunnelURI(t *testing.T, ca []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rds-ca.pem")
	if err := os.WriteFile(path, ca, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return "postgres://master@" + testRDSHost + ":15432/jobcron?sslmode=verify-full&hostaddr=127.0.0.1&sslrootcert=" + url.QueryEscape(path)
}

func TestVerifiedTunnelTLS(t *testing.T) {
	ca, cert := tunnelCertificate(t, testRDSHost)
	raw := tunnelURI(t, ca)
	private, err := migrationDatabaseURL(raw, "synthetic-private-password")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := verifiedTunnelConfig(private)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "127.0.0.1" || cfg.Port != 15432 || cfg.TLSConfig.ServerName != testRDSHost || cfg.TLSConfig.InsecureSkipVerify || cfg.TLSConfig.RootCAs == nil || len(cfg.Fallbacks) != 0 {
		t.Fatal("tunnel lost loopback binding or hostname/CA verification")
	}
	if len(cfg.RuntimeParams) != 0 {
		t.Fatal("hostaddr leaked into startup parameters")
	}
	issuer, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	wrongName := *issuer
	wrongName.SerialNumber = big.NewInt(2)
	wrongName.IsCA = false
	wrongName.DNSNames = []string{"other.rds.amazonaws.com"}
	key := cert.PrivateKey.(*ecdsa.PrivateKey)
	wrongDER, err := x509.CreateCertificate(rand.Reader, &wrongName, issuer, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	mismatched := tls.Certificate{Certificate: [][]byte{wrongDER, cert.Certificate[0]}, PrivateKey: key}
	for _, test := range []struct {
		name string
		cert tls.Certificate
		pass bool
	}{
		{"valid", cert, true},
		{"untrusted", func() tls.Certificate { _, c := tunnelCertificate(t, testRDSHost); return c }(), false},
		{"hostname mismatch", mismatched, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			clientConfig := cfg.TLSConfig.Clone()
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			server := tls.Server(right, &tls.Config{Certificates: []tls.Certificate{test.cert}})
			finished := make(chan error, 1)
			go func() { finished <- server.HandshakeContext(ctx) }()
			err := tls.Client(left, clientConfig).HandshakeContext(ctx)
			if (err == nil) != test.pass {
				t.Fatalf("handshake success=%v, want %v", err == nil, test.pass)
			}
			left.Close()
			right.Close()
			<-finished
		})
	}
	for _, address := range []string{"remote.example:15432", "127.0.0.1:5432", "[::1]:15432"} {
		if _, err := cfg.DialFunc(context.Background(), "tcp", address); err == nil {
			t.Fatal("non-tunnel dial accepted")
		}
	}
	if _, err := cfg.DialFunc(context.Background(), "unix", "127.0.0.1:15432"); err == nil {
		t.Fatal("unix dial accepted")
	}
	if _, err := cfg.LookupFunc(context.Background(), testRDSHost); err == nil {
		t.Fatal("remote lookup accepted")
	}
	registered, release, err := registerTunnelDatabase(private, true)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if strings.Contains(registered, "synthetic-private-password") || strings.Contains(registered, testRDSHost) {
		t.Fatal("registry handle discloses connection")
	}
}

func TestVerifiedTunnelRejectsUnsafeInputs(t *testing.T) {
	ca, _ := tunnelCertificate(t, testRDSHost)
	raw := tunnelURI(t, ca)
	for _, unsafe := range []string{
		strings.Replace(raw, "15432", "0", 1), strings.Replace(raw, "15432", "65536", 1),
		strings.Replace(raw, "15432", "+15432", 1), strings.Replace(raw, "hostaddr=127.0.0.1", "hostaddr=10.0.0.1", 1),
		strings.Replace(raw, testRDSHost, "127.0.0.1", 1), raw + "&sslmode=require", raw + "&sslinsecure=1",
		strings.Replace(raw, "sslrootcert=", "sslrootcert=relative", 1),
	} {
		_, err := migrationDatabaseURL(unsafe, "secret-no-disclosure")
		if err == nil {
			t.Fatal("unsafe coordinates accepted")
		}
		if strings.Contains(err.Error(), "secret-no-disclosure") || strings.Contains(err.Error(), unsafe) {
			t.Fatal("private input disclosed")
		}
	}
	for _, content := range [][]byte{nil, []byte("not a certificate")} {
		private, err := migrationDatabaseURL(tunnelURI(t, content), "secret-no-disclosure")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := verifiedTunnelConfig(private); err == nil {
			t.Fatal("invalid CA accepted")
		}
	}
	u, _ := url.Parse(raw)
	path := u.Query().Get("sslrootcert")
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := verifiedTunnelConfig(raw); err == nil {
		t.Fatal("writable CA accepted")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(filepath.Dir(path), "link.pem")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("sslrootcert", link)
	u.RawQuery = q.Encode()
	if _, err := verifiedTunnelConfig(u.String()); err == nil {
		t.Fatal("symlink CA accepted")
	}
	t.Setenv("PGHOST", "unapproved.invalid")
	if _, err := verifiedTunnelConfig(raw); err == nil {
		t.Fatal("ambient PG selector accepted")
	}
}

func TestProductionOperatorRequiresVerifiedCoordinates(t *testing.T) {
	legacy, _ := migrationDatabaseURL("postgres://master@127.0.0.1:15432/jobcron?sslmode=require", "private")
	if _, _, err := registerTunnelDatabase(legacy, true); err == nil {
		t.Fatal("production require-only TLS accepted")
	}
	var output bytes.Buffer
	err := run(context.Background(), []string{"migrate", "--database-url=private-argv", "--bogus"}, envMap{"JOBCRON_ENV": "production"}, nil, &output)
	if err == nil || strings.Contains(err.Error(), "private-argv") || output.Len() != 0 {
		t.Fatal("production migrate argv guard failed")
	}
}
