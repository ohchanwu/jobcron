package storage

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/ohchanwu/jobcron/internal/profile"
)

func TestPostingViewTransactionEstablishesBeforeLaterReads(t *testing.T) {
	forEachAccountStore(t, func(t *testing.T, st *Store) {
		ctx := context.Background()
		u, err := st.CreateUser(ctx, "view@example.invalid", "synthetic-hash")
		if err != nil {
			t.Fatal(err)
		}
		prof := profile.Profile{CareerYears: 0}
		pj, _ := profile.Marshal(prof)
		if _, _, err := st.SaveProfileForUser(ctx, u.ID, pj); err != nil {
			t.Fatal(err)
		}
		view, err := st.BeginPostingView(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer view.Close()
		p := samplePosting()
		p.SourcePostingID = "later"
		p.FirstSeenAt = time.Now().UTC()
		if _, _, err := st.UpsertPosting(ctx, p); err != nil {
			t.Fatal(err)
		}
		bundle, err := view.Load(ctx, "", "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.Postings) != 0 {
			t.Fatal("later posting mixed with earlier established profile snapshot")
		}
		if bundle.ProfileJSON != pj || !bundle.HasProfile {
			t.Fatal("profile lost from coherent view")
		}
		if err := view.Close(); err != nil {
			t.Fatal(err)
		}
		fresh, err := st.BeginPostingView(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer fresh.Close()
		after, err := fresh.Load(ctx, "", "", false, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(after.Postings) != 1 {
			t.Fatal("next read failed to observe committed posting")
		}
	})
}

func TestPostingViewCredentialPresenceUsesEstablishedSnapshot(t *testing.T) {
	st := newPostgresTestStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "view-presence@example.invalid", "synthetic-hash")
	if err != nil {
		t.Fatal(err)
	}
	v, err := st.BeginPostingView(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	if _, err := st.db.ExecContext(ctx, `INSERT INTO user_ai_credentials(user_id,provider,ciphertext,nonce,encryption_version) VALUES ($1,'gemini',$2,$3,1)`, u.ID, make([]byte, 32), make([]byte, 12)); err != nil {
		t.Fatal(err)
	}
	if present, err := v.CredentialPresence(ctx, "gemini"); err != nil || present {
		t.Fatalf("later credential mixed into first read: %v %v", present, err)
	}
}

func TestPostingViewPreservesDuplicateSourceOrdering(t *testing.T) {
	forEachAccountStore(t, func(t *testing.T, st *Store) {
		ctx := context.Background()
		u, err := st.CreateUser(ctx, "source-order@example.invalid", "synthetic-hash")
		if err != nil {
			t.Fatal(err)
		}
		p := samplePosting()
		canonical, _, err := st.UpsertPosting(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range []string{"rallit", "greenhouse_moloco"} {
			p.Source = source
			id, _, err := st.UpsertPosting(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.MarkDuplicate(ctx, id, canonical); err != nil {
				t.Fatal(err)
			}
		}
		previous, err := st.DuplicateSourcesByCanonical(ctx)
		if err != nil {
			t.Fatal(err)
		}
		view, err := st.BeginPostingView(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		defer view.Close()
		bundle, err := view.Load(ctx, "", "", false, false)
		if err != nil || !reflect.DeepEqual(bundle.DuplicateSources, previous) {
			t.Fatalf("duplicate-source order changed: before=%v after=%v err=%v", previous, bundle.DuplicateSources, err)
		}
	})
}
