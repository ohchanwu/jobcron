# Scraper List — v1.1+ Source Roadmap

The shipping `Scraper` interface (`internal/scraper/scraper.go`) lets us add
sources incrementally. This file tracks which portals we've evaluated, which
ones are in flight, and which we've ruled out, so the next "should we add
X?" question gets a fast answer instead of re-deriving the verdict.

For each portal: posture (legal/operational risk), data shape, and the
estimated work to add. Verdicts can change — re-evaluate when a portal's
behavior visibly shifts (new robots.txt, Cloudflare block, ToS update).

## Onboarding-friction principle

**A source that requires the user to register an account / apply for a key /
paste credentials is a non-starter for v1.x.** The whole product thesis is
"open the binary, see a calm briefing" — every setup step the user has to
do before the first scrape eats away at that promise. New sources should
be transparently scrape-able with no per-user credential setup, or they
get deferred.

This rules out anything fronted by data.go.kr (per-user OpenAPI key with a
manual application flow) and similar gated APIs.

## Triage

| Portal | Status | Why |
|---|---|---|
| **점핏** (jumpit.saramin.co.kr) | ✅ shipped (v1) | Baseline. Clean JSON API, friendly rate. |
| **랠릿** (rallit.com) | ✅ next target | Dev-focused, lots of 신입, Next.js (likely a usable internal JSON API). robots.txt permits `/positions`. **No credentials required.** |
| **프로그래머스** (career.programmers.co.kr) | ✅ after rallit | Dev-focused, friendly company posture, JSON API. Lower 신입 volume than rallit per the user's read. No credentials. |
| **로켓펀치** (rocketpunch.com) | ✅ after programmers | Startup-ecosystem signal; complements 점핏's mid-large bias. No credentials. |
| **워크넷** (work.go.kr) | ⏸ deprioritized | Code shipped in v0.2 and works, but requires each user to register at data.go.kr and paste their own OpenAPI key. That setup friction conflicts with the "open the binary, see a briefing" thesis. Left in the repo as dormant scaffolding; not registered by default, not documented in the README, not the path to push on next. |
| **Direct company pages** (Toss, 당근, 배민, Naver, Kakao, Coupang…) | ✅ later phase | One scraper each, shipped one per release. Companies want their careers page indexed, so posture is friendly. Maintenance burden grows linearly. |
| **원티드** (wanted.co.kr) | ⏸ deferred | Cloudflare-blocked (returns 403 to robots.txt + bot fingerprint checks). Needs headless browser or paid proxy. |
| **사람인 main** (saramin.co.kr) | ⏸ skip | Same company as 점핏; mostly duplicate signal; defensive posture on the main site. |
| **잡코리아** (jobkorea.co.kr) | ❌ skip | Litigious history of pursuing scrapers via legal channels. |
| **LinkedIn Korea** | ❌ skip | hiQ v. LinkedIn precedent; aggressive enforcement; CAPTCHA-heavy. |
| **인크루트** (incruit.com) | ❌ skip | Low signal, defensive posture, dated UX. |

## Per-portal notes

### 랠릿 (rallit.com) — next target

- robots.txt: `Allow: /resumes`, explicit `Disallow` only on `/applicants`, `/apply`, `/auth`, `/my`. `/positions` (the listings path) is unrestricted.
- Stack: Next.js. The site URL pattern `/positions/{id}/{slug}` strongly suggests `getServerSideProps` or a `/api/positions` JSON endpoint behind it — to be confirmed with a browser-devtools recon pass.
- Public B2B API exists at <https://inflab-1.gitbook.io/rallit>, but that is for partner companies syncing their listings INTO Rallit. Not what we want; we want to scrape OUT.
- 신입 filter likely lives on a URL query param (TBD — `?career=NEWCOMER` or similar).
- Implementation effort: ~1-2 days assuming the JSON endpoint is discoverable.
- Open question: do they fingerprint/throttle anonymous scrapers? Hit at 1 req/s first and see.

### 프로그래머스 (career.programmers.co.kr) — after rallit

- Dev-only portal, owned by Grepp (the coding-test company). They have a developer-friendly culture.
- Listing URL: `https://career.programmers.co.kr/job` with filter URL params. Looks like a Next.js / React SPA backed by a JSON API.
- Lower 신입 volume than rallit per user's general impression — still worth adding for the company-specific listings rallit lacks.
- robots.txt + ToS need a recon pass before implementation.

### 로켓펀치 (rocketpunch.com)

- Startup-community focus, lots of seed-to-Series-B roles 점핏 underweights.
- Combination job board + LinkedIn-style profiles. Listings live at `/jobs`.
- Has a long-standing community-permissive posture but ToS should be re-read at implementation time.

### Direct company career pages

Each is its own scraper, ship one per release. High-value candidates ranked by 신입 hiring volume + the cohort the user actually wants to target:

1. **토스** (toss.im) — careers.toss.im — high 신입 demand, JSON-friendly
2. **당근** (daangn.com) — about.daangn.com/jobs — fast hiring
3. **우아한형제들** (woowahan.com) — career.woowahan.com — 배민
4. **카카오** (kakao.com) — careers.kakao.com — 공채 cycles dominate
5. **네이버** (navercorp.com) — recruit.navercorp.com — 공채 cycles dominate
6. **쿠팡** (coupang.jobs) — large 신입 program

Note: companies on 공채 (batch hiring) cycles publish less continuously and
may need closer integration with the 공채-calendar feature parked in
`feature-ideas.md` to be useful.

### 원티드 (wanted.co.kr) — deferred

Blocked behind Cloudflare bot detection — returns 403 even for `/robots.txt`
when requested without a real-browser fingerprint. Options if we ever
revisit:

- Headless browser (chromedp / Playwright) — kills the single-binary, no-CGO
  thesis. Likely a non-starter for v1.x.
- Paid residential-proxy + browser-impersonation library — adds ongoing cost,
  fragile.
- Direct contact with 원티드 for an API partner key — most legitimate path
  but requires a relationship.

Stays in "would be nice, not at any cost" bucket.

## Cross-cutting concerns

### Rate limits

Every source paces at **1 request per second** as a respect convention, set
inside each scraper's `client.go`. This is more conservative than most
sources require — the goal is to be invisible to the source's ops team. The
"천천히 가져올게요" SSE notice surfaces this to the user.

If a source publishes its own documented rate limit, honor that as a ceiling
but do not exceed 1 req/s without an explicit reason captured in the
source's `API_NOTES.md`.

### Authentication model

| Portal | Auth |
|---|---|
| 점핏 | None — public endpoint with browser-shaped headers |
| 랠릿 / 프로그래머스 / 로켓펀치 | TBD — probably none for the public listings path |
| Direct company pages | None |
| 워크넷 | Per-user OpenAPI key (data.go.kr). **Disqualifying for v1.x** — see the onboarding-friction principle above. |

**Never embed a credential in the binary.** It would be extractable from the
open-source repo in seconds, and would violate the issuing portal's ToS for
every install. This is also why per-user-key sources are deferred entirely:
the only architecturally correct path forces setup friction on every user.

### Sweep behavior

The per-source `MAX(last_seen_at)` baseline in `SweepStalePostings`
(`internal/storage/postings.go`) means each source's freshness clock is
independent. A 점핏 scrape doesn't stale out 워크넷 postings and vice
versa. Same goes for any future source — no change needed when adding one.

### Profile toggle

Each registered source gets a checkbox in the profile form's `공고 출처`
fieldset. Defaults to enabled; user can mute any source they find noisy.
Bookmarked postings are exempt from the toggle (explicit user signal wins).
