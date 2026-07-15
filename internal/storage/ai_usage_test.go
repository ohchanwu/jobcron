package storage

import (
	"context"
	"testing"
)

func TestAIUsageLedger(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	const day = "2026-06-03"

	t.Run("absent day reads as zero", func(t *testing.T) {
		in, out, err := st.AIUsageForDay(ctx, testAIUserID, day)
		if err != nil || in != 0 || out != 0 {
			t.Fatalf("empty day = (%d,%d) err=%v, want (0,0)", in, out, err)
		}
	})

	t.Run("accumulates across calls (survives a simulated restart)", func(t *testing.T) {
		if err := st.AddAIUsage(ctx, testAIUserID, day, 100, 20); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		if err := st.AddAIUsage(ctx, testAIUserID, day, 50, 10); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		in, out, err := st.AIUsageForDay(ctx, testAIUserID, day)
		if err != nil {
			t.Fatalf("AIUsageForDay: %v", err)
		}
		if in != 150 || out != 30 {
			t.Fatalf("ledger = (%d,%d), want (150,30) — increments must accumulate", in, out)
		}
	})

	t.Run("days are independent", func(t *testing.T) {
		if err := st.AddAIUsage(ctx, testAIUserID, "2026-06-04", 7, 3); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		in, out, _ := st.AIUsageForDay(ctx, testAIUserID, "2026-06-04")
		if in != 7 || out != 3 {
			t.Fatalf("day 2 = (%d,%d), want (7,3) — must not bleed across days", in, out)
		}
		// Day 1 is untouched.
		in1, _, _ := st.AIUsageForDay(ctx, testAIUserID, day)
		if in1 != 150 {
			t.Fatalf("day 1 input = %d, want 150 (unchanged)", in1)
		}
	})

	t.Run("month aggregate sums only matching UTC month", func(t *testing.T) {
		if err := st.AddAIUsage(ctx, testAIUserID, "2026-06-30", 11, 1); err != nil {
			t.Fatalf("AddAIUsage previous month: %v", err)
		}
		if err := st.AddAIUsage(ctx, testAIUserID, "2026-07-01", 20, 2); err != nil {
			t.Fatalf("AddAIUsage month day 1: %v", err)
		}
		if err := st.AddAIUsage(ctx, testAIUserID, "2026-07-31", 30, 3); err != nil {
			t.Fatalf("AddAIUsage month day 31: %v", err)
		}
		if err := st.AddAIUsage(ctx, testAIUserID, "2026-08-01", 99, 9); err != nil {
			t.Fatalf("AddAIUsage next month: %v", err)
		}
		in, out, err := st.AIUsageForMonth(ctx, testAIUserID, "2026-07")
		if err != nil {
			t.Fatalf("AIUsageForMonth: %v", err)
		}
		if in != 50 || out != 5 {
			t.Fatalf("month aggregate = (%d,%d), want (50,5)", in, out)
		}
	})
}

func TestAIUsageIsIsolatedByUserAndMonth(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	userA := insertAIStorageTestUser(t, st, "ai-usage-a@example.invalid")
	userB := insertAIStorageTestUser(t, st, "ai-usage-b@example.invalid")

	for _, usage := range []struct {
		userID int64
		day    string
		input  int
		output int
	}{
		{userID: userA, day: "2026-07-01", input: 100, output: 10},
		{userID: userA, day: "2026-07-31", input: 200, output: 20},
		{userID: userB, day: "2026-07-01", input: 7, output: 3},
		{userID: userB, day: "2026-08-01", input: 900, output: 90},
	} {
		if err := st.AddAIUsage(ctx, usage.userID, usage.day, usage.input, usage.output); err != nil {
			t.Fatalf("AddAIUsage user=%d day=%s: %v", usage.userID, usage.day, err)
		}
	}

	for _, test := range []struct {
		name       string
		userID     int64
		wantInput  int
		wantOutput int
	}{
		{name: "user A", userID: userA, wantInput: 300, wantOutput: 30},
		{name: "user B", userID: userB, wantInput: 7, wantOutput: 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, output, err := st.AIUsageForMonth(ctx, test.userID, "2026-07")
			if err != nil || input != test.wantInput || output != test.wantOutput {
				t.Fatalf("AIUsageForMonth = (%d,%d) err=%v, want (%d,%d)", input, output, err, test.wantInput, test.wantOutput)
			}
		})
	}

	input, output, err := st.AIUsageForDay(ctx, userB, "2026-07-01")
	if err != nil || input != 7 || output != 3 {
		t.Fatalf("user B day usage = (%d,%d) err=%v, want (7,3)", input, output, err)
	}
}

func TestAIUsageRejectsNonPositiveUserID(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	if err := st.AddAIUsage(ctx, 0, "2026-07-15", 1, 1); err == nil {
		t.Fatal("AddAIUsage accepted userID 0")
	}
	if _, _, err := st.AIUsageForDay(ctx, -1, "2026-07-15"); err == nil {
		t.Fatal("AIUsageForDay accepted a negative userID")
	}
	if _, _, err := st.AIUsageForMonth(ctx, 0, "2026-07"); err == nil {
		t.Fatal("AIUsageForMonth accepted userID 0")
	}
}
