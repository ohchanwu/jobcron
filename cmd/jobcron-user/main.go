// Command jobcron-user manages production app user accounts.
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

	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/storage"
	"golang.org/x/term"
)

type envMap map[string]string

func main() {
	if err := runWithPrompt(context.Background(), os.Args[1:], environMap(os.Environ()), os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, env envMap, in io.Reader, out io.Writer) error {
	return runWithPrompt(ctx, args, env, in, out, out)
}

func runWithPrompt(ctx context.Context, args []string, env envMap, in io.Reader, out, promptOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: jobcron-user create-owner|reset-password|delete-user --database-url URL --email EMAIL")
	}
	switch args[0] {
	case "create-owner":
		return runOwnerCommand(ctx, args[0], args[1:], env, in, out, promptOut, false)
	case "reset-password":
		return runOwnerCommand(ctx, args[0], args[1:], env, in, out, promptOut, true)
	case "delete-user":
		return runDeleteUserCommand(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runOwnerCommand(ctx context.Context, name string, args []string, env envMap, in io.Reader, out, promptOut io.Writer, reset bool) error {
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
	email = auth.NormalizeEmail(email)
	if err := auth.ValidateEmail(email); err != nil {
		return err
	}
	passwordEnv, passwordLabel := "JOBCRON_OWNER_PASSWORD", "Owner"
	if reset {
		passwordEnv, passwordLabel = "JOBCRON_USER_PASSWORD", "User"
	}
	password, err := commandPassword(env, passwordEnv, passwordLabel, in, promptOut)
	if err != nil {
		return err
	}
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	st, err := openUserStore(databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()

	var user storage.User
	if reset {
		user, err = st.ResetUserPassword(ctx, email, passwordHash)
	} else {
		user, err = st.CreateOwnerUser(ctx, email, passwordHash)
	}
	if err != nil {
		return err
	}
	if reset {
		fmt.Fprintf(out, "reset password for %s (user ID %d)\n", user.Email, user.ID)
	} else {
		fmt.Fprintf(out, "created owner user %s (user ID %d)\n", user.Email, user.ID)
	}
	return nil
}

func runDeleteUserCommand(ctx context.Context, args []string, out io.Writer) error {
	var databaseURL, email, confirmEmail string
	fs := flag.NewFlagSet("delete-user", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&databaseURL, "database-url", "", "PostgreSQL database URL")
	fs.StringVar(&email, "email", "", "user email address")
	fs.StringVar(&confirmEmail, "confirm-email", "", "repeat the user email address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if databaseURL == "" {
		return errors.New("user: --database-url is required")
	}
	if email == "" {
		return errors.New("user: --email is required")
	}
	if confirmEmail == "" {
		return errors.New("user: --confirm-email is required")
	}
	email = auth.NormalizeEmail(email)
	confirmEmail = auth.NormalizeEmail(confirmEmail)
	if err := auth.ValidateEmail(email); err != nil {
		return err
	}
	if confirmEmail != email {
		return errors.New("user: email confirmation does not match")
	}

	st, err := openUserStore(databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	user, found, err := st.UserByEmail(ctx, email)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("user: user does not exist")
	}
	deleted, err := st.DeleteUser(ctx, user.ID)
	if err != nil {
		return err
	}
	if !deleted {
		return errors.New("user: user no longer exists")
	}
	fmt.Fprintf(out, "deleted user %s (user ID %d)\n", user.Email, user.ID)
	return nil
}

func openUserStore(databaseURL string) (*storage.Store, error) {
	st, err := storage.OpenPostgres(databaseURL)
	if err != nil {
		return nil, errors.New("user: open PostgreSQL database")
	}
	return st, nil
}

func commandPassword(env envMap, envName, label string, in io.Reader, out io.Writer) (string, error) {
	if password := env[envName]; password != "" {
		return password, nil
	}
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "%s password: ", label)
	if file, ok := in.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		password, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("user: read %s password: %w", strings.ToLower(label), err)
		}
		if len(password) == 0 {
			return "", fmt.Errorf("user: %s password is required", strings.ToLower(label))
		}
		return string(password), nil
	}
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("user: read %s password: %w", strings.ToLower(label), err)
	}
	password := strings.TrimRight(line, "\r\n")
	if password == "" {
		return "", fmt.Errorf("user: %s password is required", strings.ToLower(label))
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
