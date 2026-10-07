package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/storage"
)

func ownerEmailFile(t *testing.T, value string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "owner-email")
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCreateOwnerEmailFileNormalizesBeforePassword(t *testing.T) {
	for _, production := range []bool{false, true} {
		env := envMap{"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, " OWNER@EXAMPLE.COM \n", 0400)}
		args := []string{"create-owner"}
		if production {
			env["JOBCRON_ENV"] = "production"
			env["DATABASE_URL_FILE"] = ownerEmailFile(t, "unused", 0600)
		} else {
			args = append(args, "--database-url", "unused")
		}
		value, err := ownerEmailInput(env, "", false)
		if err != nil || auth.NormalizeEmail(value) != "owner@example.com" {
			t.Fatal("file email normalization changed")
		}
		var out bytes.Buffer
		err = run(context.Background(), args, env, strings.NewReader("\n"), &out)
		if err == nil || err.Error() != "user: owner password is required" || out.String() != "Owner password: " {
			t.Fatalf("file email did not reach password validation: %v", err)
		}
	}
}

func TestCreateOwnerEmailRejectsUnsafeInputsWithoutDisclosure(t *testing.T) {
	const privateEmail = "private-owner@example.com"
	valid := ownerEmailFile(t, privateEmail, 0600)
	link := filepath.Join(t.TempDir(), "email-link")
	if err := os.Symlink(valid, link); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		env  envMap
		args []string
	}{
		{"symlink", envMap{"JOBCRON_OWNER_EMAIL_FILE": link}, nil},
		{"public mode", envMap{"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, privateEmail, 0644)}, nil},
		{"missing file", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid + ".missing"}, nil},
		{"relative file", envMap{"JOBCRON_OWNER_EMAIL_FILE": "private-email-file"}, nil},
		{"invalid email", envMap{"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, "private-invalid-email", 0600)}, nil},
		{"newline", envMap{"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, privateEmail+"\nsecond", 0600)}, nil},
		{"empty", envMap{"JOBCRON_OWNER_EMAIL_FILE": ownerEmailFile(t, "", 0600)}, nil},
		{"literal plus file", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid, "JOBCRON_OWNER_EMAIL": privateEmail}, nil},
		{"empty literal plus file", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid, "JOBCRON_OWNER_EMAIL": ""}, nil},
		{"flag plus file", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid}, []string{"--email", privateEmail}},
		{"empty flag plus file", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid}, []string{"--email="}},
		{"flag plus literal", envMap{"JOBCRON_OWNER_EMAIL": privateEmail}, []string{"--email", privateEmail}},
		{"duplicate flag", envMap{}, []string{"--email", privateEmail, "--email=other@example.com"}},
		{"parse error", envMap{}, []string{"--unknown=" + privateEmail}},
		{"positional", envMap{"JOBCRON_OWNER_EMAIL_FILE": valid}, []string{privateEmail}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"create-owner", "--database-url", "unused"}, tc.args...)
			in := strings.NewReader("unused-password\n")
			var out bytes.Buffer
			err := run(context.Background(), args, tc.env, in, &out)
			if err == nil || in.Len() != len("unused-password\n") || out.Len() != 0 {
				t.Fatal("unsafe email accepted or reached password/database work")
			}
			for _, secret := range []string{privateEmail, valid, link, "private-invalid-email", "private-email-file"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("email error disclosed input")
				}
			}
		})
	}
}

func TestCreateOwnerEmailProductionOutputIsValueBlind(t *testing.T) {
	var out bytes.Buffer
	user := storage.User{ID: 42, Email: "private-owner@example.com"}
	writeOwnerCreated(&out, user, true)
	if out.String() != "owner_user_ready=true user_id=42\n" {
		t.Fatal("production owner output disclosed identity or changed receipt")
	}
	out.Reset()
	writeOwnerCreated(&out, user, false)
	if out.String() != "created owner user private-owner@example.com (user ID 42)\n" {
		t.Fatal("nonproduction output compatibility changed")
	}
}

func TestProductionCreateOwnerRequiresEmailFile(t *testing.T) {
	path := ownerEmailFile(t, "unused", 0600)
	for _, tc := range []struct {
		args []string
		env  envMap
	}{
		{nil, envMap{}},
		{nil, envMap{"JOBCRON_OWNER_EMAIL": "private-owner@example.com"}},
		{[]string{"--email", "private-owner@example.com"}, envMap{}},
		{[]string{"--", "--email=private-owner@example.com"}, envMap{}},
		{[]string{"positional", "-email=private-owner@example.com"}, envMap{}},
	} {
		tc.env["JOBCRON_ENV"] = "production"
		tc.env["DATABASE_URL_FILE"] = path
		var out bytes.Buffer
		err := run(context.Background(), append([]string{"create-owner"}, tc.args...), tc.env, strings.NewReader("unused-password\n"), &out)
		if err == nil || err.Error() != "user: production requires JOBCRON_OWNER_EMAIL_FILE" || out.Len() != 0 {
			t.Fatalf("production email restriction failed: %v", err)
		}
	}
}
