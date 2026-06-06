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

## Per-tenant URL strategy (verified live 2026-06-06)

`absolute_url` is **not** uniformly trustworthy, so each tenant declares a
`LinkStrategy`:

| Tenant   | Strategy       | Click-through URL                                   | Why |
|----------|----------------|-----------------------------------------------------|-----|
| daangn   | `LinkSite`     | `team.daangn.com/jobs/{id}/`                        | `absolute_url` is a dead `about.daangn.com?gh_jid=` marketing link (2026-05-27 audit). |
| krafton  | `LinkAbsolute` | `job-boards.greenhouse.io/krafton/jobs/{id}`        | `absolute_url` is the hosted board page; renders the job directly (200, no redirect). |
| moloco   | `LinkAbsolute` | `job-boards.greenhouse.io/moloco/jobs/{id}`         | same as krafton. |
| sendbird | `LinkBoard`    | `job-boards.greenhouse.io/sendbird/jobs/{id}`       | `absolute_url` is a custom `sendbird.com/careers?gh_jid=` page; the canonical hosted board 302-redirects there with the `gh_jid` deep-link intact. The canonical URL is stored because it's stable and Greenhouse-owned. |

`integration_test.go`'s `TestLiveGreenhouseURLsResolve` GETs each posting URL
(following redirects, browser UA) and asserts the destination contains the
posting id — a regression guard against the dead-link / wrong-redirect class
of bug.

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
`Sendbird()`, registered individually in `cmd/job-scraper/main.go`.

- **Ship:** `daangn`, `krafton` (AI Research interns + 신입 tracks — strongest),
  `moloco` (Seoul ML/SWE interns), `sendbird` (Seoul AI engineer interns).
- **Not shipped:** `toss` runs a custom host (`api-public.toss.im`), not the
  standard `boards-api.greenhouse.io` — needs separate handling.
- **Excluded:** senior-only Korean boards (`coupang` / `coupanginternal` /
  `seoulrobotics`) — high Korea volume, ~0 신입 dev.

Adding a tenant is a one-function change in `tenant.go` + one `sources.go`
label + one `main.go` registration. Always browser-verify the click-through
URL for the new token before shipping.
