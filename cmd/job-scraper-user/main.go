// Command job-scraper-user manages production app user accounts.
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ohchanwu/job-scraper/internal/auth"
	"github.com/ohchanwu/job-scraper/internal/storage"
)

type envMap map[string]string

func main() {
	if err := run(context.Background(), os.Args[1:], environMap(os.Environ()), os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, env envMap, in io.Reader, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: job-scraper-user create-owner|reset-password --database-url URL --email EMAIL")
	}
	switch args[0] {
	case "create-owner":
		return runOwnerCommand(ctx, args[0], args[1:], env, in, out, false)
	case "reset-password":
		return runOwnerCommand(ctx, args[0], args[1:], env, in, out, true)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runOwnerCommand(ctx context.Context, name string, args []string, env envMap, in io.Reader, out io.Writer, reset bool) error {
	var databaseURL, email string
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&databaseURL, "database-url", "", "PostgreSQL database URL")
	fs.StringVar(&email, "email", "", "owner email address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if databaseURL == "" {
		return errors.New("user: --database-url is required")
	}
	if email == "" {
		return errors.New("user: --email is required")
	}
	password, err := ownerPassword(env, in, out)
	if err != nil {
		return err
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	st, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	var user storage.User
	if reset {
		user, err = st.ResetOwnerPassword(ctx, email, passwordHash)
	} else {
		user, err = st.CreateOwnerUser(ctx, email, passwordHash)
	}
	if err != nil {
		return err
	}
	if reset {
		fmt.Fprintf(out, "reset owner password for %s\n", user.Email)
	} else {
		fmt.Fprintf(out, "created owner user %s\n", user.Email)
	}
	return nil
}

func ownerPassword(env envMap, in io.Reader, out io.Writer) (string, error) {
	if password := env["JOBSCRAPER_OWNER_PASSWORD"]; password != "" {
		return password, nil
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprint(out, "Owner password: ")
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("user: read owner password: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", errors.New("user: owner password is required")
	}
	return password, nil
}

func environMap(environ []string) envMap {
	env := make(envMap, len(environ))
	for _, item := range environ {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}
