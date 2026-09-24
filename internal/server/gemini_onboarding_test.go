package server

import (
	"context"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/auth"
	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestUnsavedProfileDefaultsToRecommendedGemini(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /profile status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	gemini := strings.Index(body, `value="gemini" selected`)
	anthropic := strings.Index(body, `value="anthropic"`)
	if gemini < 0 {
		t.Fatalf("unsaved profile did not preselect Gemini:\n%s", body)
	}
	if anthropic < 0 || gemini >= anthropic {
		t.Fatalf("Gemini option must precede Anthropic: gemini=%d anthropic=%d", gemini, anthropic)
	}
	if !strings.Contains(body, `value="gemini-3.5-flash-lite" selected`) {
		t.Fatalf("unsaved profile did not preselect Gemini default model:\n%s", body)
	}
	if _, _, found, err := st.Profile(context.Background()); err != nil || found {
		t.Fatalf("rendering defaults must not create a profile row: found=%v err=%v", found, err)
	}
}

func TestSavedAISelectionsRemainTruthful(t *testing.T) {
	tests := []struct {
		name       string
		profile    profile.Profile
		wantSelect string
		wantModel  string
	}{
		{name: "explicit AI off", profile: profile.Profile{JobLikes: "preserve me"}, wantSelect: `<option value="" selected>없음 (끄기)</option>`},
		{name: "explicit Anthropic", profile: profile.Profile{JobLikes: "preserve me", AIProvider: "anthropic", AIModel: "claude-sonnet-4-6"}, wantSelect: `value="anthropic" selected`, wantModel: `value="claude-sonnet-4-6" selected`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, st := newTestServer(t, &fakeScraper{})
			wantJSON, err := profile.Marshal(tt.profile)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := st.SaveProfile(context.Background(), wantJSON); err != nil {
				t.Fatal(err)
			}

			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantSelect) || tt.wantModel != "" && !strings.Contains(body, tt.wantModel) {
				t.Fatalf("saved selection not rendered truthfully:\n%s", body)
			}
			gotJSON, _, found, err := st.Profile(context.Background())
			if err != nil || !found || gotJSON != wantJSON {
				t.Fatalf("GET changed saved profile: found=%v err=%v got=%q want=%q", found, err, gotJSON, wantJSON)
			}
		})
	}
}

func TestProfileGeminiOnboardingCopyAndSafeLinks(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/profile", nil))
	body := rec.Body.String()

	for _, want := range []string{
		"추천: Google Gemini",
		"무과금 API 등급(unpaid API tier)",
		"2~5분",
		"Google이 제공 여부와 사용 한도를 결정",
		`href="https://aistudio.google.com/app/apikey" target="_blank" rel="noopener noreferrer"`,
		`<span class="sr-only">새 탭에서 열림</span>`,
		`href="/guides/gemini-api-key"`,
		"프롬프트와 응답을 제품 개선에 사용할 수",
		"이력서나 민감한 정보, 기밀 정보, 개인 식별 정보",
		"암호화",
		"Google AI Studio에서 폐기",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("profile onboarding missing %q", want)
		}
	}
	if strings.Contains(body, "연결 테스트") || strings.Contains(body, "/api/ai/test") {
		t.Fatal("profile must not add a dedicated connection test")
	}
}

func TestGeminiAPIKeyGuideIsCompleteAndNoSyntheticTestRouteExists(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/guides/gemini-api-key", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET guide status = %d, want 200; body=%q", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Gemini API 키 설정 가이드",
		`<section class="guide-callout" aria-label="Gemini 무료 등급 안내">`,
		"Gemini에는 시작하기 좋은 무료 API 등급이 있습니다. gemini-3.5-flash-lite 기준 약 하루 500회의 API 요청을 사용할 수 있어요. 같은 Google 프로젝트를 Jobcron에서만 사용한다면 사용 한도에 걸릴 가능성은 낮아요. 같은 프로젝트의 API 키를 다른 용도로 함께 쓰면 한도에 도달할 수 있어요. 드물게 한도에 도달하면 다음 할당량 초기화까지 기다리거나 유료 등급이 연결된 프로젝트의 키를 사용하세요.",
		"무료 등급에서는 Google이 프롬프트와 응답을 제품 개선에 사용할 수 있어요. AI 프로필 입력란에 민감, 기밀, 개인 식별 정보를 넣을 때 주의하세요.",
		"Google AI Studio에 로그인",
		"API 키 만들기",
		`class="guide-subnote guide-subnote-optional"`,
		"프로젝트가 없을 때만 프로젝트를 만들거나 선택하라는 안내가 표시돼요.",
		`class="guide-subnote guide-subnote-advisory"`,
		"채팅, 메모, 화면 캡처에 남기지 마세요.",
		"Flash-Lite",
		"첫 실제 AI 평가",
		"잘못된 키",
		"HTTP 429",
		"모델",
		"지역",
		`href="https://aistudio.google.com/app/apikey" target="_blank" rel="noopener noreferrer"`,
		`href="https://aistudio.google.com/app/usage" target="_blank" rel="noopener noreferrer"`,
		`<span class="sr-only">새 탭에서 열림</span>`,
		`href="/profile"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("guide missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"동영상 없이도 이 안내만 따라 첫 평가까지 완료할 수 있어요. Google 화면의 이름이나 위치는 바뀔 수 있지만 필요한 단계는 같아요.",
		"시작하기 전에",
		"영구 제공이나 고정 요청 수를 보장하지 않아요",
		"키 보관·교체·폐기",
		"이 요청이 저장한 키, 선택 모델, Google 엔드포인트, Jobcron의 정상 요청 경로를 함께 확인하는 유일한 연결 확인이에요.",
		"별도의 연결 테스트 버튼이나 요청은 없어요.",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("guide unexpectedly contains removed copy %q", unwanted)
		}
	}

	stepsStart := strings.Index(body, `<ol class="guide-steps">`)
	stepsEnd := strings.Index(body, `</ol>`)
	if stepsStart < 0 || stepsEnd < stepsStart {
		t.Fatal("guide setup steps list missing")
	}
	steps := body[stepsStart:stepsEnd]
	if got := strings.Count(steps, "<li>"); got != 6 {
		t.Fatalf("numbered setup steps = %d, want 6 (project selection must be an unnumbered subnote)", got)
	}
	if !strings.Contains(steps, `<li><strong>API 키 만들기</strong>를 선택하세요.`) ||
		!strings.Contains(steps, `<p class="guide-subnote guide-subnote-optional">프로젝트가 없을 때만`) {
		t.Fatal("project guidance must be nested beneath the API key creation step")
	}
	if !strings.Contains(steps, `<li>표시된 API 키를 복사하세요.`) ||
		!strings.Contains(steps, `<p class="guide-subnote guide-subnote-advisory">채팅, 메모, 화면 캡처에 남기지 마세요.</p>`) {
		t.Fatal("key handling warning must be an advisory subnote beneath the copy step")
	}

	for _, path := range []string{"/api/ai/test", "/api/ai/connection-test", "/profile/ai-key/test"} {
		t.Run(path, func(t *testing.T) {
			testRec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(testRec, httptest.NewRequest(http.MethodPost, path, nil))
			if testRec.Code != http.StatusNotFound {
				t.Fatalf("synthetic test route %s status=%d, want 404", path, testRec.Code)
			}
		})
	}
}

func TestGeminiGuideMobileHeaderStylesAreScopedAndAllowWrapping(t *testing.T) {
	srv, _ := newTestServer(t, &fakeScraper{})

	guideRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(guideRec, httptest.NewRequest(http.MethodGet, "/guides/gemini-api-key", nil))
	guide := guideRec.Body.String()
	for _, want := range []string{`<header class="guide-header">`, `class="guide-title"`} {
		if !strings.Contains(guide, want) {
			t.Fatalf("guide header missing scoped hook %q", want)
		}
	}

	stylesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(stylesRec, httptest.NewRequest(http.MethodGet, "/static/styles.css", nil))
	styles := stylesRec.Body.String()
	for _, want := range []string{
		`.guide-main { margin-top: 1.5rem; max-width: 48rem; font-size: 1rem; }`,
		`.guide-main h2 { margin: 0 0 0.75rem; font-family: var(--serif); font-size: 1.45rem; font-weight: 500; }`,
		`.guide-subnote {`,
		`overflow-wrap: anywhere`,
		`.guide-subnote-optional`,
		`.guide-subnote-advisory`,
		`.guide-header .guide-title { min-width: 0; }`,
		`.guide-header h1`,
		`white-space: normal`,
		`.guide-header .head-right { flex: 0 0 auto; }`,
	} {
		if !strings.Contains(styles, want) {
			t.Errorf("mobile overflow regression: styles missing %q", want)
		}
	}

	subnoteStart := strings.Index(styles, `.guide-subnote {`)
	if subnoteStart < 0 {
		t.Fatal("guide subnote style block missing")
	}
	subnoteEnd := strings.Index(styles[subnoteStart:], `}`)
	if subnoteEnd < 0 {
		t.Fatal("guide subnote style block missing closing brace")
	}
	subnoteStyles := styles[subnoteStart : subnoteStart+subnoteEnd]
	if !strings.Contains(subnoteStyles, `color: var(--ink);`) {
		t.Fatalf("guide subnote must use the high-contrast ink token; block=%q", subnoteStyles)
	}
	if strings.Contains(subnoteStyles, `color: var(--ink-soft);`) {
		t.Fatalf("guide subnote must not use the low-contrast ink-soft token; block=%q", subnoteStyles)
	}

	troubleshootingStart := strings.Index(styles, `.guide-troubleshooting dd { margin:`)
	if troubleshootingStart < 0 {
		t.Fatal("guide troubleshooting description style block missing")
	}
	troubleshootingEnd := strings.Index(styles[troubleshootingStart:], `}`)
	if troubleshootingEnd < 0 {
		t.Fatal("guide troubleshooting description style block missing closing brace")
	}
	troubleshootingStyles := styles[troubleshootingStart : troubleshootingStart+troubleshootingEnd]
	if !strings.Contains(troubleshootingStyles, `color: var(--ink);`) {
		t.Fatalf("guide troubleshooting descriptions must use the high-contrast ink token; block=%q", troubleshootingStyles)
	}
	if strings.Contains(troubleshootingStyles, `color: var(--ink-soft);`) {
		t.Fatalf("guide troubleshooting descriptions must not use the low-contrast ink-soft token; block=%q", troubleshootingStyles)
	}
}

func TestProductionGeminiGuideAuthCSRFAndReadOnlyContract(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	srv.SetProductionMode(true)
	ctx := context.Background()
	hash := "$argon2id$v=19$m=65536,t=3,p=2$HnaitXE81jwvEnc/8ZDBNQ$bSyeYlt4Gm57RgICVNGJDc9qXFyISc+SkuiTHec9BQM"
	user, err := st.CreateOwnerUser(ctx, "guide@example.invalid", hash)
	if err != nil {
		t.Fatalf("CreateOwnerUser: %v", err)
	}
	const sessionValue = "guide-session-token"
	if err := st.CreateSession(ctx, user.ID, auth.HashSessionToken(sessionValue), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	unauth := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/guides/gemini-api-key", nil))
	if unauth.Code != http.StatusSeeOther || unauth.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous guide status=%d location=%q, want 303 /login", unauth.Code, unauth.Header().Get("Location"))
	}

	const csrfCookieValue = "guide-csrf-cookie"
	sessionCookie := &http.Cookie{Name: sessionCookieName, Value: sessionValue}
	getReq := httptest.NewRequest(http.MethodGet, "/guides/gemini-api-key", nil)
	getReq.AddCookie(sessionCookie)
	getReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfCookieValue})
	authenticated := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authenticated, getReq)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated guide status=%d, want 200; body=%q", authenticated.Code, authenticated.Body.String())
	}
	wantToken := html.EscapeString(srv.csrfToken(csrfCookieValue, sessionValue))
	if !strings.Contains(authenticated.Body.String(), `action="/logout"`) ||
		!strings.Contains(authenticated.Body.String(), `name="csrf_token" value="`+wantToken+`"`) {
		t.Fatal("guide logout did not receive session-bound CSRF state")
	}

	postReq := httptest.NewRequest(http.MethodPost, "/guides/gemini-api-key", nil)
	postReq.AddCookie(sessionCookie)
	postReq.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfCookieValue})
	postReq.Header.Set(csrfHeaderName, srv.csrfToken(csrfCookieValue, sessionValue))
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST guide status=%d, want 405 (no state-changing guide route)", postRec.Code)
	}
}
