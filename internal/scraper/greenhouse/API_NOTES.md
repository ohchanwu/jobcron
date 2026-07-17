# Greenhouse multi-tenant board API — notes

The `greenhouse` package scrapes companies that host their careers board on
Greenhouse's public, no-auth board API. One board = one company, so each
registered `Scraper` wraps a single `Tenant` (own `Source()`, badge, and
profile toggle); the Greenhouse plumbing is shared. 당근 was the first tenant
(it pioneered this path as a standalone package); krafton / moloco / sendbird
followed on 2026-06-06.

## API shape

```
GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs?content=true
→ { "jobs": [ { id, title, absolute_url, location:{name}, content,
               first_published, application_deadline, metadata:[...] }, ... ],
    "meta": { "total": N } }
```

- **No auth.** `?content=true` returns the full HTML body + metadata in one
  call, so `FetchDetail` is a no-op.
- `content` is HTML-escaped (`&lt;`, `&amp;`, `&nbsp;`); `html.UnescapeString`
  before `stripHTML`.
- `metadata` is `[{name, value, value_type}]`; `value` is `any` (bool for
  `yes_no`, string for `single_select`). **The field set is per-board** — 당근
  carries `Engineer` / `Prior Experience`; krafton carries `Job Category - *`
  buckets; moloco/sendbird carry generic fields. Do **not** rely on any
  metadata field being present across tenants.
- robots.txt on `boards-api.greenhouse.io` only disallows `/embed/`; the
  `/v1/boards/` path is unrestricted.

## Per-tenant URL strategy (daangn/krafton/moloco verified 2026-06-06; sendbird re-verified + fixed 2026-06-08)

`absolute_url` is **not** uniformly trustworthy, so each tenant declares a
`LinkStrategy`:

| Tenant   | Strategy       | Click-through URL                                          | Why |
|----------|----------------|-----------------------------------------------------------|-----|
| daangn   | `LinkSite`     | `careers.daangn.com/jobs/role/{id}/`                       | `absolute_url` is a dead `about.daangn.com?gh_jid=` marketing link (2026-05-27 audit). |
| krafton  | `LinkAbsolute` | `job-boards.greenhouse.io/krafton/jobs/{id}`              | `absolute_url` is the hosted board page; renders the job directly (200, no redirect). |
| moloco   | `LinkAbsolute` | `job-boards.greenhouse.io/moloco/jobs/{id}`              | same as krafton. |
| sendbird | `LinkSiteJob`   | `sendbird.com/job/{id}`                                          | **Was `LinkBoard`; regressed 2026-06-08.** BOTH the `absolute_url` (`sendbird.com/careers?gh_jid=`) AND the hosted board URL (`/sendbird/jobs/{id}`) now 302-redirect to sendbird.com's careers **front page** — the deep-link is ignored. Greenhouse's `embed/job_app` view loads but shows ONLY the apply form (no JD). Only `sendbird.com/job/{id}` renders the actual posting with its JD (the URL their own careers front page links to). **Caveat:** that page is a client-rendered SPA that returns a soft HTTP **404** to non-browser clients — it renders fine in a real browser but a raw `curl`/`http.Get` sees a 404 shell. So it cannot be validated by the raw-GET URL-resolve test (which shape-checks it and skips the fetch) — **browser-verify 센드버드 by hand.** sendbird.com robots.txt allows `/job/`. |

`integration_test.go`'s `TestLiveGreenhouseURLsResolve` GETs each posting URL
(following redirects, browser UA) and asserts (a) the **final URL stays on the
expected host** (Greenhouse for board/embed/absolute strategies; the tenant's
own site for `LinkSite`) AND (b) the body contains the posting id. The host
assertion was added 2026-06-08: id-only was a false-pass because the sendbird
careers front page echoes the `gh_jid`. **When adding/auditing a tenant, this
test must be run live — each tenant's click-through can break independently.**

## 신입 detection — two strategies

Greenhouse has **no structured 신입 field** (unlike 점핏 `career=0`, 데모데이
`experience_level`, 그리팅 `careerType`), so detection is per-tenant:

- **`DetectMetadata` (당근 only).** `Engineer == true` AND `Prior Experience`
  contains `신입`. Reliable; specific to 당근's board config. Byte-identical to
  the old standalone daangn scraper.
- **`DetectHeuristic` (everyone else).** A title/description heuristic in
  `classify.go`. A posting is kept iff it is **Korea-based** (these companies
  are multinational; we only want their Korean roles), is a **dev role**
  (`scraper.HasDevKeyword` over title+description — the broad 신입-dev scope:
  SWE / 보안 / 데이터 / DevOps·인프라 / AI / QA), is **newcomer-marked in the
  title** (`신입` / `인턴` / `junior` / `경력무관` / …), is **not explicitly
  senior**, and carries **no ≥2-year experience floor** unless it is an
  internship.

The heuristic trades recall for precision: an unmarked "Backend Engineer" open
to 신입 is missed (the Greenhouse 신입-dev count is a documented **lower
bound**), but the briefing is not flooded with senior roles.

### Heuristic validation — live audit (2026-06-08)

Both unvalidated guesses in `classify.go` were audited against all four boards'
live listings. **Both kept; the trade is validated, not changed.**

**Title-only newcomer rule (`classifyHeuristic`).** The question was whether to
widen the newcomer check from the title to the body (a 신입 role stated only in
the body is missed). Counting by hand every Korea-based dev role whose *body*
contains a newcomer word but whose *title* does not:

| board | total jobs | Korea dev | kept | body-only newcomer hits | real 신입 among them |
|---|---|---|---|---|---|
| krafton | 64 | 27 | 6 | 16 | **0** |
| moloco | 67 | 20 | 1 | 0 | 0 |
| sendbird | 17 | 6 | 1 | 1 | **0** |
| daangn | 38 | (metadata) | 3 | — | — |

**Every** body-only hit was boilerplate, not a real 신입 signal:

- **krafton (16/16 false):** 15 match `신입` inside the identical application-
  instructions footer every krafton JD carries — *"신입일 경우 자기소개서를, 경력일
  경우 경력기술서를 중심으로…"* — and 1 matches `수습` inside *"5개월의 수습기간을
  적용합니다"* (a probation clause). All 16 are 5년+/7년+/2년+/postdoc roles.
- **sendbird (1/1 false):** "Software Engineer, AI Agent" matched `junior` inside
  *"You enjoy mentoring junior engineers"* — the exact senior-mentions-juniors
  trap the title-only rule exists to dodge.

So widening to the body would inject **17 senior/experienced false positives and
zero real 신입 roles**. A senior-marker guard wouldn't save it (Server Programmer
(5년 이상) has no senior *title* marker), and the ≥2-year guard misses the
exp-parse-fails cases (postdoc, the sendbird SWE). **Verdict: keep title-only.**

**당근 신입/경력 → max 3 (`metadataBounds`).** The `3` is harmless for the target
audience: a 신입 user has `CareerYears == 0`, and `scoreCareer`'s first branch
`(newcomer && years == 0)` short-circuits to the full award *before* `maxCareer`
is ever read — so 3 vs 5 vs 10 changes no 신입 score. When an AI extraction
exists it overrides `p.MaxCareer` outright. The 3 only sets the displayed range
and the near-miss boundary for *experienced* users (out of scope). **Verdict:
keep 3.**

### Gotchas baked into the heuristic

- **`intern` ⊄ `Internal`.** English newcomer/intern markers are
  word-boundary anchored (`\bintern\b`), or "Director, **Internal**
  Communications" would match. Korean markers (`인턴`/`신입`/`주니어`) are safe as
  substrings.
- **Contradictory signals.** "Jr. 팀원 (3~6년 / 계약직)" carries a weak junior
  word *and* a real 3–6-year floor — the ≥2-year reject drops it. Interns are
  exempt because an intern JD's "2년 이상" describes research background, not a
  career floor.
- **Location filter.** Multinational boards list non-Korea offices (moloco:
  Menlo Park; krafton: Amsterdam). A posting open to Seoul among several cities
  still matches.

## Curated token list

Tokens live in `tenant.go` as `Daangn()` / `Krafton()` / `Moloco()` /
`Sendbird()`, registered individually in `cmd/jobcron/main.go`.

- **Ship:** `daangn`, `krafton` (AI Research interns + 신입 tracks — strongest),
  `moloco` (Seoul ML/SWE interns), `sendbird` (Seoul AI engineer interns).
- **Not shipped:** `toss` runs a custom host (`api-public.toss.im`), not the
  standard `boards-api.greenhouse.io` — needs separate handling.
- **Excluded:** senior-only Korean boards (`coupang` / `coupanginternal` /
  `seoulrobotics`) — high Korea volume, ~0 신입 dev.

Adding a tenant is a one-function change in `tenant.go` + one `sources.go`
label + one `main.go` registration. Always browser-verify the click-through
URL for the new token before shipping.
