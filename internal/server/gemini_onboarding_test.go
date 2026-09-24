package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		"Google AI Studio에 로그인",
		"프로젝트를 만들거나 선택",
		"API 키 만들기",
		"Flash-Lite",
		"첫 실제 AI 평가",
		"별도의 연결 테스트",
		"잘못된 키",
		"HTTP 429",
		"모델",
		"지역",
		"폐기",
		"암호화",
		"프롬프트와 응답",
		`href="https://aistudio.google.com/app/apikey" target="_blank" rel="noopener noreferrer"`,
		`href="https://aistudio.google.com/app/usage" target="_blank" rel="noopener noreferrer"`,
		`href="/profile"`,
		"동영상 없이도",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("guide missing %q", want)
		}
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
