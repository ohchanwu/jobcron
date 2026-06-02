package ai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseScoreDeltaValid(t *testing.T) {
	raw := `{"items":[
		{"signal":"백엔드 중심","kind":"presence","delta":6,"quote":"서버 개발자를 찾습니다","matched_goal":"좋아하는 업무"},
		{"signal":"재택 없음","kind":"absence","delta":-4,"forms":["재택","원격"],"matched_goal":"단기 목표"}
	]}`
	items, err := parseScoreDelta([]byte(raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].Kind != KindPresence || items[0].Delta != 6 || items[0].Quote != "서버 개발자를 찾습니다" {
		t.Fatalf("presence item = %+v", items[0])
	}
	if items[1].Kind != KindAbsence || items[1].Delta != -4 || len(items[1].Forms) != 2 {
		t.Fatalf("absence item = %+v", items[1])
	}
}

func TestParseScoreDeltaDropsBadItems(t *testing.T) {
	t.Run("unknown kind dropped, rest kept", func(t *testing.T) {
		raw := `{"items":[
			{"signal":"x","kind":"sideeffect","delta":99,"quote":"무시"},
			{"signal":"y","kind":"presence","delta":5,"quote":"좋은 회사 문화"}
		]}`
		items, err := parseScoreDelta([]byte(raw))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if len(items) != 1 || items[0].Kind != KindPresence {
			t.Fatalf("expected only the presence item, got %+v", items)
		}
	})

	t.Run("zero delta dropped", func(t *testing.T) {
		items, _ := parseScoreDelta([]byte(`{"items":[{"signal":"x","kind":"presence","delta":0,"quote":"아무거나"}]}`))
		if len(items) != 0 {
			t.Fatalf("a zero-delta item must be dropped, got %+v", items)
		}
	})

	t.Run("empty items array is valid", func(t *testing.T) {
		items, err := parseScoreDelta([]byte(`{"items":[]}`))
		if err != nil || len(items) != 0 {
			t.Fatalf("empty array: items=%+v err=%v", items, err)
		}
	})

	t.Run("tolerates markdown fences", func(t *testing.T) {
		raw := "결과:\n```json\n{\"items\":[{\"signal\":\"s\",\"kind\":\"presence\",\"delta\":3,\"quote\":\"원격 근무 가능\"}]}\n```"
		items, err := parseScoreDelta([]byte(raw))
		if err != nil || len(items) != 1 {
			t.Fatalf("fenced: items=%+v err=%v", items, err)
		}
	})
}

func TestParseScoreDeltaRejects(t *testing.T) {
	cases := map[string]string{
		"malformed JSON": `{"items":[`,
		"no object":      `nothing parseable here`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseScoreDelta([]byte(raw)); err == nil {
				t.Fatalf("expected rejection for %s", name)
			}
		})
	}
}

func TestGateDeltaPresence(t *testing.T) {
	sent := "제목: 신입 백엔드\n회사: 가나다\n\n서버 개발자를 찾습니다. 재택근무 가능하고 좋은 회사 문화를 갖췄습니다."

	t.Run("real quote survives", func(t *testing.T) {
		raw := []RawDeltaItem{{Signal: "백엔드", Kind: KindPresence, Delta: 6, Quote: "서버 개발자를 찾습니다"}}
		d := GateDelta(raw, sent, sent)
		if len(d.Items) != 1 || d.NetDelta != 6 {
			t.Fatalf("expected one survivor netting 6, got %+v", d)
		}
		if d.Items[0].Evidence != "서버 개발자를 찾습니다" {
			t.Fatalf("evidence = %q, want the verbatim quote", d.Items[0].Evidence)
		}
	})

	t.Run("absent quote dropped", func(t *testing.T) {
		raw := []RawDeltaItem{{Signal: "x", Kind: KindPresence, Delta: 6, Quote: "주 4일 근무 보장"}}
		d := GateDelta(raw, sent, sent)
		if len(d.Items) != 0 || d.NetDelta != 0 {
			t.Fatalf("a quote not in the text must be dropped, got %+v", d)
		}
	})

	t.Run("below char floor dropped", func(t *testing.T) {
		// "재택" is in the text but only 2 chars — below the 6-rune floor.
		raw := []RawDeltaItem{{Signal: "x", Kind: KindPresence, Delta: 6, Quote: "재택"}}
		if d := GateDelta(raw, sent, sent); len(d.Items) != 0 {
			t.Fatalf("quote below char floor must be dropped, got %+v", d)
		}
	})

	t.Run("below token floor dropped", func(t *testing.T) {
		// A single long token clears the char floor but not the ≥2-token floor.
		s := "백엔드개발자포지션입니다"
		raw := []RawDeltaItem{{Signal: "x", Kind: KindPresence, Delta: 6, Quote: "백엔드개발자포지션입니다"}}
		if d := GateDelta(raw, s, s); len(d.Items) != 0 {
			t.Fatalf("single-token quote must be dropped, got %+v", d)
		}
	})

	t.Run("injected common word dropped", func(t *testing.T) {
		// The classic injection foothold: a filler word that's trivially present.
		raw := []RawDeltaItem{{Signal: "x", Kind: KindPresence, Delta: 50, Quote: "및"}}
		if d := GateDelta(raw, sent, sent); len(d.Items) != 0 {
			t.Fatalf("a filler word must not satisfy the gate, got %+v", d)
		}
	})

	t.Run("presence checks the SENT text, not the full Description (S5)", func(t *testing.T) {
		// The model could only have seen the truncated sent text; a quote that
		// lives only past the truncation point must not be honored.
		truncated := "제목: 신입 백엔드\n회사: 가나다"
		full := truncated + "\n\n주 4일 근무를 보장합니다"
		raw := []RawDeltaItem{{Signal: "x", Kind: KindPresence, Delta: 8, Quote: "주 4일 근무를 보장합니다"}}
		if d := GateDelta(raw, truncated, full); len(d.Items) != 0 {
			t.Fatalf("a quote only in the untruncated tail must be dropped, got %+v", d)
		}
	})
}

func TestGateDeltaAbsence(t *testing.T) {
	full := "제목: 백엔드 개발자\n\n온사이트 근무입니다. 주말 근무는 없습니다."

	t.Run("all forms absent → penalty survives with code-verified evidence", func(t *testing.T) {
		raw := []RawDeltaItem{{Signal: "재택 불가", Kind: KindAbsence, Delta: -5, Forms: []string{"재택", "원격", "remote"}}}
		d := GateDelta(raw, full, full)
		if len(d.Items) != 1 || d.NetDelta != -5 {
			t.Fatalf("expected one absence penalty netting -5, got %+v", d)
		}
		want := "'재택/원격/remote' 등 관련 언급 없음 (코드 확인)"
		if d.Items[0].Evidence != want {
			t.Fatalf("evidence = %q, want %q", d.Items[0].Evidence, want)
		}
	})

	t.Run("one present form drops the whole penalty (fail-safe)", func(t *testing.T) {
		raw := []RawDeltaItem{{Signal: "근무 형태", Kind: KindAbsence, Delta: -5, Forms: []string{"재택", "온사이트"}}}
		if d := GateDelta(raw, full, full); len(d.Items) != 0 {
			t.Fatalf("a present surface form must drop the penalty, got %+v", d)
		}
	})

	t.Run("absence checks the FULL Description, not the truncated sent text (S5)", func(t *testing.T) {
		// The form sits past the truncation point as a standalone token: the
		// model (seeing only the truncated text) thinks it's absent, but our
		// absence check reads the full Description and must NOT apply the penalty.
		// (Token-exact: the form must appear as its own token — "재택" would not
		// match the compound "재택근무", the documented 개발/개발자 tradeoff.)
		sentTrunc := "제목: 백엔드 개발자"
		full := sentTrunc + "\n\n재택 근무 가능합니다"
		raw := []RawDeltaItem{{Signal: "재택 불가", Kind: KindAbsence, Delta: -6, Forms: []string{"재택"}}}
		if d := GateDelta(raw, sentTrunc, full); len(d.Items) != 0 {
			t.Fatalf("a form present past truncation must not read as absent, got %+v", d)
		}
	})

	t.Run("empty / blank forms dropped", func(t *testing.T) {
		for _, forms := range [][]string{nil, {}, {"  "}, {"!!!"}} {
			raw := []RawDeltaItem{{Signal: "x", Kind: KindAbsence, Delta: -5, Forms: forms}}
			if d := GateDelta(raw, full, full); len(d.Items) != 0 {
				t.Fatalf("unconfirmable forms %v must drop the penalty, got %+v", forms, d)
			}
		}
	})
}

func TestGateDeltaNetsPositivesAndNegatives(t *testing.T) {
	full := "제목: 백엔드\n\n서버 개발자를 찾습니다. 온사이트 근무."
	raw := []RawDeltaItem{
		{Signal: "백엔드", Kind: KindPresence, Delta: 6, Quote: "서버 개발자를 찾습니다"},
		{Signal: "재택 불가", Kind: KindAbsence, Delta: -4, Forms: []string{"재택", "원격"}},
		{Signal: "헛소리", Kind: KindPresence, Delta: 9, Quote: "있지도 않은 문구"}, // dropped: not in text
	}
	d := GateDelta(raw, full, full)
	if len(d.Items) != 2 {
		t.Fatalf("expected 2 survivors, got %d (%+v)", len(d.Items), d.Items)
	}
	if d.NetDelta != 2 { // +6 - 4
		t.Fatalf("net = %d, want 2 (the dropped +9 must not count)", d.NetDelta)
	}
	if d.Stale {
		t.Fatal("GateDelta must leave Stale false; the merge sets it")
	}
}

// TestGateTokenizeInvariants locks the gate's tokenizer to the same behavior
// scoring/match.go's tokenize guarantees. internal/ai cannot import scoring, so
// this copy is verified independently — keep it in lockstep with match_test.go.
func TestGateTokenizeInvariants(t *testing.T) {
	t.Run("개발 does not match 개발자 (token-exact)", func(t *testing.T) {
		if tokenSubsequence("개발자를 찾습니다", "개발") {
			t.Fatal("개발 must NOT be a subsequence of 개발자 — distinct tokens")
		}
	})
	t.Run("phrase order is preserved", func(t *testing.T) {
		if tokenSubsequence("백엔드 신입 개발자", "개발자 신입") {
			t.Fatal("token order must matter")
		}
		if !tokenSubsequence("백엔드 신입 개발자", "신입 개발자") {
			t.Fatal("contiguous in-order tokens must match")
		}
	})
	t.Run("whitespace and newlines are separators", func(t *testing.T) {
		if !tokenSubsequence("서버\n\n개발자", "서버 개발자") {
			t.Fatal("the \\n\\n section join must tokenize the same as a space")
		}
	})
	t.Run("case-insensitive", func(t *testing.T) {
		if !tokenSubsequence("React Native 개발", "react native") {
			t.Fatal("matching must be case-insensitive")
		}
	})
	t.Run("empty phrase matches nothing", func(t *testing.T) {
		if tokenSubsequence("아무 텍스트", "   ") {
			t.Fatal("a phrase that tokenizes to nothing must match nothing")
		}
	})
}

// TestHTTPProviderScoreDeltaEndToEnd exercises the wired ScoreDelta: complete →
// parseScoreDelta against a canned Anthropic-shaped response.
func TestHTTPProviderScoreDeltaEndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		// The posting and profile must both reach the model in one user message.
		if !strings.Contains(string(body), "채용 공고") || !strings.Contains(string(body), "지원자의 목표") {
			t.Errorf("request body missing the posting/profile headings: %s", body)
		}
		io.WriteString(w, `{"content":[{"type":"text","text":"{\"items\":[{\"signal\":\"백엔드\",\"kind\":\"presence\",\"delta\":6,\"quote\":\"서버 개발자\",\"matched_goal\":\"좋아하는 업무\"}]}"}],"usage":{"input_tokens":120,"output_tokens":40}}`)
	}))
	defer srv.Close()

	p, err := newHTTPProvider(anthropicSpec, "sk", "claude-x", srv.URL, 0)
	if err != nil {
		t.Fatalf("newHTTPProvider: %v", err)
	}
	items, usage, err := p.ScoreDelta(context.Background(), "제목: 신입 백엔드", "좋아하는 업무: 서버 개발")
	if err != nil {
		t.Fatalf("ScoreDelta: %v", err)
	}
	if len(items) != 1 || items[0].Quote != "서버 개발자" || items[0].Delta != 6 {
		t.Fatalf("items = %+v", items)
	}
	if usage.InputTokens != 120 || usage.OutputTokens != 40 {
		t.Fatalf("usage = %+v", usage)
	}
}
