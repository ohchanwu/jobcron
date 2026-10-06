package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/ai"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestAIPerCallCapProfileFlow(t *testing.T) {
	t.Run("unsaved profile", func(t *testing.T) {
		srv, st := newTestServer(t, &fakeScraper{})
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "비우면 기본 200.") {
			t.Fatalf("unsaved profile must render the 200 default: status=%d", rec.Code)
		}
		if _, _, found, err := st.Profile(context.Background()); err != nil || found {
			t.Fatalf("rendering default created a profile: found=%v err=%v", found, err)
		}
	})

	for _, tc := range []struct {
		name      string
		jsonField string
		formValue string
		wantRaw   int
		wantCap   int
	}{
		{name: "absent", wantCap: 200},
		{name: "implicit submitted default", formValue: "200", wantCap: 200},
		{name: "zero", jsonField: `,"ai_per_call_cap":0`, formValue: "0", wantCap: 200},
		{name: "negative retains fallback", jsonField: `,"ai_per_call_cap":-1`, formValue: "-1", wantRaw: -1, wantCap: 200},
		{name: "explicit old default", jsonField: `,"ai_per_call_cap":50`, formValue: "50", wantRaw: 50, wantCap: 50},
		{name: "explicit 100", jsonField: `,"ai_per_call_cap":100`, formValue: "100", wantRaw: 100, wantCap: 100},
		{name: "explicit 200", jsonField: `,"ai_per_call_cap":200`, formValue: "200", wantRaw: 200, wantCap: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, st := newPostgresTestServer(t, &fakeScraper{})
			srv.SetProductionMode(true)
			ctx := context.Background()
			userID, cookie := createSessionUser(t, st, "cap-flow@example.invalid", "synthetic-cap-flow-session")
			before := `{"ai_provider":"anthropic","ai_model":"claude-sonnet-4-6"` + tc.jsonField + `}`
			if _, _, err := st.SaveProfileForUser(ctx, userID, before); err != nil {
				t.Fatal(err)
			}
			cipher := newAIRuntimeTestCipher(t, 0x63)
			srv.SetCredentialCipher(cipher)
			saveAIRuntimeCredential(t, st, cipher, userID, "anthropic", "synthetic-cap-flow-key")
			srv.newAIProvider = func(provider, _, _ string, _ time.Duration) (ai.Provider, error) {
				return &fingerprintProvider{name: provider}, nil
			}

			assertRuntime := func() {
				t.Helper()
				runtime, err := srv.aiRuntimeForUser(ctx, userID)
				if err != nil || runtime == nil {
					t.Fatalf("runtime = %v, err = %v", runtime, err)
				}
				if runtime.PerCallCap != tc.wantCap {
					t.Errorf("runtime cap = %d, want %d", runtime.PerCallCap, tc.wantCap)
				}
				if runtime.RunTokenCap != aiRunTokenCapForUSDCents(30) ||
					runtime.DailyTokenCap != minPositive(1_000_000, aiDailyTokenCapForUSDCents(50)) ||
					runtime.MonthlyTokenCap != aiMonthlyTokenCapForUSDCents(1_000) {
					t.Errorf("unrelated token budgets changed: %+v", runtime)
				}
			}
			assertForm := func(wantValue string) {
				t.Helper()
				req := httptest.NewRequest(http.MethodGet, "/profile", nil)
				req.AddCookie(cookie)
				rec := httptest.NewRecorder()
				srv.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusOK {
					t.Fatalf("GET /profile = %d: %s", rec.Code, rec.Body)
				}
				body := rec.Body.String()
				field := regexp.MustCompile(`<input\b[^>]*name="ai_per_call_cap"[^>]*>`).FindString(body)
				if !strings.Contains(body, "한 번에 분석할 공고 수") ||
					!strings.Contains(field, `value="`+wantValue+`"`) ||
					!strings.Contains(field, `placeholder="`+strconv.Itoa(tc.wantCap)+`"`) ||
					!strings.Contains(field, `min="1"`) {
					t.Errorf("cap field = %s, want value %q, effective cap %d and min 1", field, wantValue, tc.wantCap)
				}
			}
			initialValue := tc.formValue
			if initialValue == "0" || tc.jsonField == "" {
				initialValue = ""
			}
			assertForm(initialValue)
			assertRuntime()
			if got, _, found, err := st.ProfileForUser(ctx, userID); err != nil || !found || got != before {
				t.Fatalf("read paths rewrote profile: found=%v err=%v got=%q want=%q", found, err, got, before)
			}

			form := url.Values{
				"ai_provider": {"anthropic"}, "ai_model": {"claude-sonnet-4-6"},
				"ai_per_call_cap": {tc.formValue},
			}
			req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(cookie)
			addCSRFToRequest(req, srv, cookie)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/briefing" {
				t.Fatalf("POST /profile = %d: %s", rec.Code, rec.Body)
			}
			gotJSON, _, found, err := st.ProfileForUser(ctx, userID)
			if err != nil || !found {
				t.Fatalf("read saved profile: found=%v err=%v", found, err)
			}
			got, err := profile.Unmarshal(gotJSON)
			if err != nil {
				t.Fatal(err)
			}
			if got.AIPerCallCap != tc.wantRaw {
				t.Errorf("saved cap = %d, want %d", got.AIPerCallCap, tc.wantRaw)
			}
			if tc.wantRaw == 0 && strings.Contains(gotJSON, "ai_per_call_cap") {
				t.Errorf("default cap must remain omitted from canonical JSON: %s", gotJSON)
			}
			wantValue := ""
			if tc.wantRaw != 0 {
				wantValue = strconv.Itoa(tc.wantRaw)
			}
			assertForm(wantValue)
			assertRuntime()
		})
	}
}
