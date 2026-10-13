package server

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"sync/atomic"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestPostingReadPathsAfterRestartHaveZeroEffects(t *testing.T) {
	original, st := newPostgresTestServer(t, &fakeScraper{})
	original.SetProductionMode(true)
	uid, cookie := createSessionUser(t, st, "read-counters@example.invalid", "read-counters-synthetic-session")
	prof := profile.Profile{AIProvider: "anthropic", AIModel: ai.DefaultModel("anthropic"), JobLikes: "synthetic current goal"}
	saveAIRuntimeProfile(t, st, uid, prof)
	cipher := &countingCipher{inner: newAIRuntimeTestCipher(t, 0x72)}
	saveAIRuntimeCredential(t, st, cipher, uid, "anthropic", "counter-synthetic-key")
	// This deliberately removes encrypted columns in the OWNED schema. Every
	// native page/observer query must still succeed: a hidden ciphertext read
	// cannot be made to look safe merely because a decrypt counter stayed zero.
	if _, err := st.SQLDB().Exec(`ALTER TABLE user_ai_credentials DROP COLUMN ciphertext, DROP COLUMN nonce`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SQLDB().Exec(`CREATE TABLE observer_mutations (table_name TEXT NOT NULL);
 CREATE FUNCTION observer_audit_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN INSERT INTO observer_mutations VALUES (TG_TABLE_NAME); RETURN NULL; END $$;`); err != nil {
		t.Fatal(err)
	}
	tables, err := st.SQLDB().Query(`SELECT tablename FROM pg_tables WHERE schemaname=current_schema() AND tablename NOT IN ('observer_mutations','schema_migrations') ORDER BY tablename`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := tables.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if _, err := st.SQLDB().Exec(fmt.Sprintf(`CREATE TRIGGER observer_audit AFTER INSERT OR UPDATE OR DELETE ON %q FOR EACH ROW EXECUTE FUNCTION observer_audit_write()`, name)); err != nil {
			t.Fatal(err)
		}
	}
	var factories atomic.Int64
	stub := rerateStub()
	restarted := New(st, &fakeScraper{})
	restarted.SetProductionMode(true)
	restarted.SetCredentialCipher(cipher)
	restarted.newAIProvider = func(string, string, string, time.Duration) (ai.Provider, error) { factories.Add(1); return stub, nil }
	request := func(path string, expected int) {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		restarted.Handler().ServeHTTP(w, r)
		if w.Code != expected {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
	}
	for _, path := range []string{"/briefing", "/bookmarks", "/?sort=date", "/?sort=score"} {
		request(path, 200)
	}
	for _, mode := range []string{"surface=today&sort=score", "surface=bookmarks&sort=saved", "surface=archive&sort=date", "surface=archive&sort=score"} {
		request("/api/posting-view?"+mode+"&generation=1", 200)
		request("/api/posting-view?"+mode+"&generation=2", 200)
	}
	// Two real subscribe/reconnect windows, no operation-starting route and no
	// Last-Event-ID replay assumption. Canceling must release the listener.
	server := httptest.NewServer(restarted.Handler())
	defer server.Close()
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/posting-updates", nil)
		r.AddCookie(cookie)
		r.Header.Set("Last-Event-ID", "old-epoch")
		response, err := server.Client().Do(r)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		scan := bufio.NewScanner(response.Body)
		if !scan.Scan() || scan.Text() != "event: dirty" {
			cancel()
			response.Body.Close()
			t.Fatal("no flushed initial resync")
		}
		cancel()
		response.Body.Close()
	}
	var mutations int
	if err := st.SQLDB().QueryRow(`SELECT COUNT(*) FROM observer_mutations`).Scan(&mutations); err != nil {
		t.Fatal(err)
	}
	if mutations != 0 || factories.Load() != 0 || cipher.opens.Load() != 0 || stub.ScoreDeltaCalls != 0 || stub.Calls != 0 || stub.ValidateDealbreakersCalls != 0 {
		t.Fatalf("read effects: SQL=%d factories=%d decrypts=%d stage1=%d stage1b=%d stage2=%d", mutations, factories.Load(), cipher.opens.Load(), stub.Calls, stub.ValidateDealbreakersCalls, stub.ScoreDeltaCalls)
	}
	// Positive control: the owner-chosen legacy exception remains real and
	// separate, not suppressed. Only sessions activity may be written here.
	request("/api/briefing-status", 200)
	request("/api/rerate/status?surface=archive", 200)
	var activity int
	if err := st.SQLDB().QueryRow(`SELECT COUNT(*) FROM observer_mutations WHERE table_name='sessions'`).Scan(&activity); err != nil || activity == 0 {
		t.Fatal("legacy activity positive control failed", err)
	}
	var other int
	if err := st.SQLDB().QueryRow(`SELECT COUNT(*) FROM observer_mutations WHERE table_name<>'sessions'`).Scan(&other); err != nil || other != 0 {
		t.Fatal("legacy status wrote more than activity", other, err)
	}
	if factories.Load() != 0 || cipher.opens.Load() != 0 {
		t.Fatal("legacy status resolved credentials")
	}
	t.Logf("native windows: document/projection/snapshot/subscribe/reconnect SQL=0 factory=0 decrypt=0 provider=0; unsuppressed legacy status activity=%d other writes=0; encrypted columns absent", activity)
}
