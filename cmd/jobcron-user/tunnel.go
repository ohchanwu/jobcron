package main

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var rdsHostname = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+\.rds\.amazonaws\.com$`)

func validTunnelPort(port string) bool {
	if port == "" || strings.Trim(port, "0123456789") != "" {
		return false
	}
	n, err := strconv.ParseUint(port, 10, 16)
	return err == nil && n > 0
}

func validateVerifiedTunnel(u *url.URL, q url.Values) error {
	if !rdsHostname.MatchString(u.Hostname()) || !validTunnelPort(u.Port()) ||
		len(q) != 3 || len(q["sslmode"]) != 1 || q.Get("sslmode") != "verify-full" ||
		len(q["hostaddr"]) != 1 || q.Get("hostaddr") != "127.0.0.1" ||
		len(q["sslrootcert"]) != 1 || !filepath.IsAbs(q.Get("sslrootcert")) {
		return errors.New("user: verified TLS requires an RDS hostname, explicit loopback tunnel port and CA file")
	}
	return nil
}

// The URI uses libpq's host/hostaddr split. pgx does not implement hostaddr;
// remove it and register a config with a fixed loopback dial while retaining
// the real RDS TLS ServerName. No process-global resolver or storage changes.
func verifiedTunnelConfig(databaseURL string) (*pgx.ConnConfig, error) {
	invalid := errors.New("user: invalid verified database configuration")
	u, err := url.Parse(databaseURL)
	if err != nil || u.Scheme != "postgres" || u.Opaque != "" || u.Fragment != "" || u.User == nil {
		return nil, invalid
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil || validateVerifiedTunnel(u, q) != nil {
		return nil, invalid
	}
	// pgx inherits libpq environment settings, including service overrides.
	// Reject these rather than permitting an ambient tunnel/TLS selector.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "PG") {
			return nil, errors.New("user: verified operator connection forbids ambient PG settings")
		}
	}
	if err := checkCAFile(q.Get("sslrootcert")); err != nil {
		return nil, invalid
	}
	q.Del("hostaddr")
	u.RawQuery = q.Encode()
	cfg, err := pgx.ParseConfig(u.String())
	if err != nil {
		return nil, invalid
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || cfg.TLSConfig.RootCAs == nil ||
		cfg.TLSConfig.ServerName != u.Hostname() || len(cfg.Fallbacks) != 0 {
		return nil, invalid
	}
	cfg.TLSConfig.MinVersion = tls.VersionTLS12
	cfg.Host = "127.0.0.1"
	target := net.JoinHostPort(cfg.Host, u.Port())
	cfg.DialFunc = func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != target {
			return nil, errors.New("user: rejected non-tunnel database dial")
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp4", target)
	}
	cfg.LookupFunc = func(_ context.Context, host string) ([]string, error) {
		if host != "127.0.0.1" {
			return nil, errors.New("user: rejected non-tunnel database lookup")
		}
		return []string{"127.0.0.1"}, nil
	}
	return cfg, nil
}

func registerTunnelDatabase(databaseURL string, production bool) (string, func(), error) {
	noop := func() {}
	u, err := url.Parse(databaseURL)
	if err == nil && u.Query().Get("sslmode") == "verify-full" {
		cfg, err := verifiedTunnelConfig(databaseURL)
		if err != nil {
			return "", noop, err
		}
		registered := stdlib.RegisterConnConfig(cfg)
		return registered, func() { stdlib.UnregisterConnConfig(registered) }, nil
	}
	if production {
		return "", noop, errors.New("user: production operator connection requires verified TLS")
	}
	return databaseURL, noop, nil
}

func operatorDatabase(env envMap, raw string, in io.Reader, out io.Writer) (string, func(), error) {
	if env["JOBCRON_ENV"] != "production" {
		return raw, func() {}, nil
	}
	password, err := commandPassword(env, "JOBCRON_DATABASE_PASSWORD", "Database", in, out)
	if err != nil {
		return "", func() {}, err
	}
	databaseURL, err := migrationDatabaseURL(raw, password)
	if err != nil {
		return "", func() {}, err
	}
	return registerTunnelDatabase(databaseURL, true)
}
