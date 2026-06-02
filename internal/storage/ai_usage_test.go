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
		in, out, err := st.AIUsageForDay(ctx, day)
		if err != nil || in != 0 || out != 0 {
			t.Fatalf("empty day = (%d,%d) err=%v, want (0,0)", in, out, err)
		}
	})

	t.Run("accumulates across calls (survives a simulated restart)", func(t *testing.T) {
		if err := st.AddAIUsage(ctx, day, 100, 20); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		if err := st.AddAIUsage(ctx, day, 50, 10); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		in, out, err := st.AIUsageForDay(ctx, day)
		if err != nil {
			t.Fatalf("AIUsageForDay: %v", err)
		}
		if in != 150 || out != 30 {
			t.Fatalf("ledger = (%d,%d), want (150,30) — increments must accumulate", in, out)
		}
	})

	t.Run("days are independent", func(t *testing.T) {
		if err := st.AddAIUsage(ctx, "2026-06-04", 7, 3); err != nil {
			t.Fatalf("AddAIUsage: %v", err)
		}
		in, out, _ := st.AIUsageForDay(ctx, "2026-06-04")
		if in != 7 || out != 3 {
			t.Fatalf("day 2 = (%d,%d), want (7,3) — must not bleed across days", in, out)
		}
		// Day 1 is untouched.
		in1, _, _ := st.AIUsageForDay(ctx, day)
		if in1 != 150 {
			t.Fatalf("day 1 input = %d, want 150 (unchanged)", in1)
		}
	})
}
