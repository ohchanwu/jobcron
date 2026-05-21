package profile

import (
	"reflect"
	"testing"
)

func TestProfileMarshalRoundTrip(t *testing.T) {
	p := Profile{
		Stacks:         []StackPref{{Name: "React", Weight: 20}, {Name: "Go", Weight: 30}},
		Location:       LocationPref{Cities: []string{"서울", "판교"}, Weight: 15, RemoteOK: true},
		CareerYears:    0,
		SalaryFloorKRW: 50_000_000,
		MaxEducation:   EducationBachelor,
		MustHave:       []string{"React"},
		Dealbreakers:   []string{"병역특례"},
	}
	s, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Unmarshal(s)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, p) {
		t.Errorf("round-trip mismatch:\n got = %+v\nwant = %+v", got, p)
	}
}

func TestProfileMarshalIsDeterministic(t *testing.T) {
	p := Profile{
		Stacks:       []StackPref{{Name: "React", Weight: 20}},
		MustHave:     []string{"a", "b"},
		MaxEducation: EducationAssociate,
	}
	first, err := Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for i := 0; i < 10; i++ {
		again, err := Marshal(p)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if again != first {
			t.Fatalf("Marshal is not deterministic:\n %q\n %q", again, first)
		}
	}
}
