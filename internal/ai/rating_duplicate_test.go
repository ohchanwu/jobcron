package ai

import (
	"reflect"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func ratingPresence(quote string, n int) RawDeltaItem {
	return RawDeltaItem{Signal: "업무", MatchedGoal: "목표", Kind: KindPresence, Quote: quote, Delta: n}
}

func TestRatingCanonicalDuplicateEvidence(t *testing.T) {
	original := ratingPresence("서버 개발 업무", 30)
	duplicate := original
	duplicate.Signal, duplicate.MatchedGoal = "다른 설명", "다른 목표"
	duplicate.Quote = norm.NFD.String("서버, 개발 업무")
	duplicate.Delta = 20
	distinct := ratingPresence("원격 협업 가능", 25)
	sent := "서버 개발 업무. 원격 협업 가능."
	first := GateDelta([]RawDeltaItem{original, duplicate, distinct}, sent, "")
	if len(first.Items) != 2 || first.NetDelta != 40 {
		t.Fatalf("duplicate inflated result: %+v", first)
	}
	var found bool
	for _, it := range first.Items {
		if it.Delta == 20 {
			found = true
			if it.Evidence != duplicate.Quote {
				t.Fatal("representative must retain the real quote")
			}
		}
	}
	if !found {
		t.Fatal("did not retain smallest bounded same-sign magnitude")
	}
	for _, permutation := range [][]RawDeltaItem{{distinct, duplicate, original}, {duplicate, original, distinct}, {original, original, duplicate, distinct}} {
		if got := GateDelta(permutation, sent, ""); !reflect.DeepEqual(got, first) {
			t.Fatalf("permutation/repetition changed result: %+v want %+v", got, first)
		}
	}
	// Opposite signs suppress only that identity, leaving distinct evidence.
	opposite := original
	opposite.Delta = -5
	conflict := GateDelta([]RawDeltaItem{original, opposite, distinct}, sent, "")
	if len(conflict.Items) != 1 || conflict.NetDelta != 25 {
		t.Fatalf("conflicting group survived: %+v", conflict)
	}
}

func TestRatingDuplicateTieUsesDeterministicRealRepresentative(t *testing.T) {
	a := ratingPresence("서버 개발 업무", 7)
	a.Signal = "a"
	b := ratingPresence("서버, 개발 업무", 7)
	b.Signal = "b"
	for _, raw := range [][]RawDeltaItem{{a, b}, {b, a}, {a, a, b}} {
		d := GateDelta(raw, "서버 개발 업무", "")
		if len(d.Items) != 1 || d.NetDelta != 7 || d.Items[0].Signal != "a" || d.Items[0].Evidence != a.Quote {
			t.Fatalf("tie/repetition changed result: %+v", d)
		}
	}
}

func TestRatingAbsenceIdentityPreservesFormBoundaries(t *testing.T) {
	a := RawDeltaItem{Signal: "a", MatchedGoal: "목표", Kind: KindAbsence, Delta: -20, Forms: []string{"Remote Work", "재택"}}
	b := a
	b.Signal = "b"
	b.Delta = -10
	b.Forms = []string{"재택", "remote, work", "재택"}
	first := GateDelta([]RawDeltaItem{a, b}, "", "서버 개발 업무")
	if len(first.Items) != 1 || first.NetDelta != -10 {
		t.Fatalf("absence form permutation/repetition inflated: %+v", first)
	}
	if got := GateDelta([]RawDeltaItem{b, a, b}, "", "서버 개발 업무"); !reflect.DeepEqual(got, first) {
		t.Fatalf("absence order unstable: %+v", got)
	}
	c := a
	c.Forms = []string{"remote", "work", "재택"}
	if got := GateDelta([]RawDeltaItem{b, c}, "", "서버 개발 업무"); len(got.Items) != 2 || got.NetDelta != -30 {
		t.Fatalf("distinct form boundaries collapsed: %+v", got)
	}
}
