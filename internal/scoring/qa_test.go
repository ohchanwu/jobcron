package scoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ohchanwu/jobcron/internal/profile"
	"github.com/ohchanwu/jobcron/internal/scraper"
)

// Step 5.5 — dealbreaker QA fixture.
//
// testdata/qa_postings.json holds 20 real 점핏 신입 postings captured live on
// 2026-05-21. Each is hand-labeled below (qaLabels) against one QA profile,
// and the test asserts Success Criterion #5: dealbreaker handling produces
// zero false negatives and at most two false positives.
//
// Hand-labeling notes (from reading the real detail text):
//   - Dealbreaker keyword "병역특례" occurs as a standalone token in exactly
//     4 postings — all genuinely structured around military-alternative
//     service. (Verified by grep against the fixture.)
//   - Education: the QA user holds a 2-3년제 (associate) degree, so every
//     posting requiring "대학교졸업(4년)" is a genuine education dealbreaker;
//     "대학졸업(2,3년)" and "무관" postings are not.
//   - "야근" is deliberately NOT a QA dealbreaker: it appears in 9 postings,
//     every one in positive context ("야근강요 안함", "야근수당", "야근 식대").
//     A naive "야근" dealbreaker would false-positive on all of them — the
//     negation limitation the design's Matching Semantics already documents.

// qaProfile is the Step 5.5 QA profile: a 신입 backend/AI developer who holds
// a 2-3년제 degree and will not take a 병역특례-structured role.
func qaProfile() profile.Profile {
	return profile.Profile{
		CareerYears:  0,
		MaxEducation: profile.EducationAssociate,
		Stacks: []profile.StackPref{
			{Name: "Python", Weight: 30},
			{Name: "React", Weight: 20},
			{Name: "AI/인공지능", Weight: 15},
		},
		Location:     profile.LocationPref{Cities: []string{"서울"}, Weight: 15},
		Dealbreakers: []string{"병역특례"},
	}
}

// qaLabel is the hand-labeled ground truth for one posting.
type qaLabel struct {
	excluded bool
	kind     string // expected DealbreakerHit.Kind: "" | "keyword" | "education"
	note     string
}

// qaLabels maps SourcePostingID -> hand-labeled expectation.
var qaLabels = map[string]qaLabel{
	"53789527": {false, "", "넥시클 — 학력 무관, no 병역특례"},
	"53786260": {true, "education", "연합인포맥스 — requires 대학교졸업(4년)"},
	"53923048": {true, "education", "테크랩스 — requires 대학교졸업(4년)"},
	"53816258": {true, "education", "무하유 — requires 대학교졸업(4년)"},
	"53776048": {false, "", "지그키 — 대학졸업(2,3년), within reach"},
	"53718113": {true, "education", "테솔로 — requires 대학교졸업(4년)"},
	"53789097": {false, "", "아이엔마케팅 — 대학졸업(2,3년), within reach"},
	"53948023": {true, "education", "에스피에이치 — requires 대학교졸업(4년)"},
	"53789512": {false, "", "넥시클 — 학력 무관, no 병역특례"},
	"53771908": {true, "education", "엠투아이코퍼레이션 — requires 대학교졸업(4년)"},
	"53789461": {false, "", "모비루스 — 대학졸업(2,3년), within reach"},
	"53819548": {true, "education", "이비즈테크 — requires 대학교졸업(4년)"},
	"53762539": {false, "", "이너버스 — 대학졸업(2,3년), within reach"},
	"53771491": {true, "keyword", "페이타랩 — 병역특례 (산업기능요원) posting"},
	"53772511": {false, "", "디엑스솔루션 — 대학졸업(2,3년), within reach"},
	"53805973": {true, "keyword", "에너자이 — 병역특례 지원 가능자"},
	"53774625": {true, "keyword", "인텍에프에이 — 전문연구요원 병역특례"},
	"53805574": {true, "keyword", "에너자이 — 병역특례 지원 가능자"},
	"53860010": {false, "", "이비즈테크 — 대학졸업(2,3년), within reach"},
	"53799628": {true, "education", "바이트사이즈 — requires 대학교졸업(4년)"},
}

func loadQAPostings(t *testing.T) []scraper.Posting {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "qa_postings.json"))
	if err != nil {
		t.Fatalf("read qa_postings.json: %v", err)
	}
	var postings []scraper.Posting
	if err := json.Unmarshal(data, &postings); err != nil {
		t.Fatalf("parse qa_postings.json: %v", err)
	}
	return postings
}

func TestQADealbreakerFixture(t *testing.T) {
	postings := loadQAPostings(t)
	if len(postings) != 20 {
		t.Fatalf("fixture has %d postings, want 20", len(postings))
	}
	prof := qaProfile()

	var falseNeg, falsePos, excluded, clean int
	for _, p := range postings {
		label, ok := qaLabels[p.SourcePostingID]
		if !ok {
			t.Errorf("posting %s (%s) has no QA label", p.SourcePostingID, p.Company)
			continue
		}
		r := scoreNoAI(p, prof)
		gotExcluded := r.Total == -1

		switch {
		case label.excluded && !gotExcluded:
			falseNeg++
			t.Errorf("FALSE NEGATIVE: %s (%s) — should be excluded [%s], but scored %d",
				p.SourcePostingID, p.Company, label.note, r.Total)
		case !label.excluded && gotExcluded:
			falsePos++
			t.Errorf("FALSE POSITIVE: %s (%s) — should be clean [%s], but was excluded by %+v",
				p.SourcePostingID, p.Company, label.note, r.DealbreakerHit)
		case label.excluded:
			excluded++
			if r.DealbreakerHit.Kind != label.kind {
				t.Errorf("%s (%s): dealbreaker kind = %q, want %q",
					p.SourcePostingID, p.Company, r.DealbreakerHit.Kind, label.kind)
			}
		default:
			clean++
		}
	}

	t.Logf("QA fixture: %d correctly excluded, %d correctly clean, %d false neg, %d false pos",
		excluded, clean, falseNeg, falsePos)

	// Success Criterion #5.
	if falseNeg != 0 {
		t.Errorf("Success Criterion #5 FAILED: %d false negatives, want 0", falseNeg)
	}
	if falsePos > 2 {
		t.Errorf("Success Criterion #5 FAILED: %d false positives, want <= 2", falsePos)
	}
}
