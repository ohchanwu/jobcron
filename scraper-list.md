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

| Portal                                                            | Status            | Why                                                                                                                                                                                                                                                                                                                 |
| ----------------------------------------------------------------- | ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **점핏** (jumpit.saramin.co.kr)                                   | ✅ shipped (v1)   | Baseline. Clean JSON API, friendly rate.                                                                                                                                                                                                                                                                            |
| **랠릿** (rallit.com)                                             | ✅ shipped        | Dev-focused, lots of 신입, JSON API at `/api/v1/position`. No credentials required.                                                                                                                                                                                                                                 |
| **네이버 careers** (recruit.navercorp.com)                        | ✅ shipped        | Single-phase JSON API at `/rcrt/loadJobList.do`. Covers the whole Naver group (NAVER, NAVER LABS, NAVER WEBTOON, NAVER Cloud, NAVER Financial, NAVER I&S). 신입 volume is small day-to-day because Naver hires 신입 mostly via 공채 cycles; scraper still captures those when they open.                            |
| **잡알리오** (job.alio.go.kr)                                     | ✅ next target    | Government-run public-sector recruit aggregator. Clean ToS (public-information mandate), no per-user credentials, listings at `/recruit.do` + detail at `/recruitView.do?idx={id}`. Unlocks the 공공기관 신입 IT cohort (전산/정보처리) the existing sources don't reach. Legacy JSP, HTML parsing instead of JSON. |
| **데모데이** (demoday.co.kr)                                      | ⏸ deferred        | Recon on 2026-05-27: data is fetched from a public Supabase project (`xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits`), not from demoday.co.kr's own API. Their robots.txt disallows `/api/` AND `/_next/` for `User-Agent: *`, so neither in-app path is polite to crawl. Hitting Supabase directly bypasses the spirit of that disallow; running a headless browser to let the page hydrate is a major architectural shift away from `net/http`-based scrapers. Skipping for now.                                                                                                                                                  |
| **그룹바이** (groupby.kr)                                         | ⏸ deferred        | Recon on 2026-05-27: the listings API (`api.groupby.kr/startup-positions`) returns clean JSON in Chromium but **404s every non-browser HTTP client** (curl, Go's `net/http`, even with the full set of browser headers including Origin/Referer/sec-ch-ua). nginx returns an empty 404 body — clear TLS-fingerprint / JA3-style bot detection. Same posture as 원티드 / 쿠팡 — out of reach without a headless browser, which this project explicitly avoids (pure-Go + single-binary distribution).                                                                                                                                                                                                                |
| **Direct company pages — others** (Toss, 당근, 배민, etc.)        | ✅ later phase    | One scraper each, shipped one per release. Companies that want their careers page indexed have friendly postures.                                                                                                                                                                                                   |
| **프로그래머스** (career.programmers.co.kr)                       | ❌ defunct        | **Service shut down 2025-04-28.** `career.programmers.co.kr` no longer resolves. Confirmed via official notice at <https://programmers.co.kr/notices/11584>. Do not revisit.                                                                                                                                        |
| **워크넷** (work.go.kr)                                           | ⏸ deprioritized   | Code shipped in v0.2 and works, but requires each user to register at data.go.kr and paste their own OpenAPI key. That setup friction conflicts with the "open the binary, see a briefing" thesis. Left in the repo as dormant scaffolding.                                                                         |
| **로켓펀치** (rocketpunch.com)                                    | ⏸ deferred        | CloudFront-fronted (403 to plain curl, needs full browser fingerprint). robots.txt explicitly `Disallow: /*.json$` blocking the Next.js data endpoints, plus a comprehensive scraper-UA blacklist. Visibly invested in keeping bots out.                                                                            |
| **카카오 careers** (careers.kakao.com)                            | ⏸ deferred        | Full SPA with all `/api/*` endpoints returning 401 — listing data needs a bootstrap auth token. Plus careers is fragmented across 6+ subsidiary subdomains (KakaoEnterprise, KakaoStyle, KakaoEnt, KakaoPaySec, KakaoBank…), each its own portal. Not worth the reverse-engineering for v1.                         |
| **쿠팡 careers** (coupang.jobs)                                   | ⏸ deferred        | Cloudflare challenge wall — returns "Attention Required" even with full browser-fidelity headers. Same blocker as 원티드 / 로켓펀치.                                                                                                                                                                                |
| **삼성 careers** (sec.wd3.myworkdayjobs.com + samsungcareers.com) | ⏸ deferred        | Public Samsung Workday tenant carries 493 jobs across 26 countries but **zero in Korea** (KR jobs live in a separate authenticated portal). samsungcareers.com/jobs returns a 500 wrapped in 200. No accessible signal for Korean 신입 IT roles.                                                                    |
| **원티드** (wanted.co.kr)                                         | ⏸ deferred        | Cloudflare-blocked (returns 403 to robots.txt + bot fingerprint checks). Needs headless browser or paid proxy.                                                                                                                                                                                                      |
| **자소설닷컴** (jasoseol.com)                                     | ⏸ deferred        | Technically clean (Next.js `__NEXT_DATA__` exposes rich data; 11,582+ active recruit URLs; mostly 대졸 신입/인턴). BUT the ToS explicitly prohibits "자동화된 수단(예, 수집로봇, 스파이더, 스크래퍼)" — same posture as 로켓펀치. Excluding for consistency, not technical reasons.                                 |
| **링커리어** (linkareer.com)                                      | ⏸ defer           | 대학생-targeted 공모전/인턴/신입 aggregator with some 공채 signal. CloudFront-fronted but currently passes plain curl. Partial redundancy with 점핏 on 대기업 IT 공채 paths. Re-evaluate when the 공채 calendar feature lands.                                                                                      |
| **디스콰이엇** (disquiet.io)                                      | ⏸ defer           | Small curated maker/PM-leaning board (~50-100 jobs). Friendly robots.txt. Real signal but low absolute volume — better as a "long tail" source in v1.2+.                                                                                                                                                            |
| **캐치** (catch.co.kr)                                            | ⏸ defer           | 진학사 operator. Similar 대졸/공채 cohort to 자소설닷컴 but smaller. Needs dedicated ToS pass before any work.                                                                                                                                                                                                      |
| **사람인 main** (saramin.co.kr)                                   | ⏸ skip            | Same company as 점핏; mostly duplicate signal; defensive posture on the main site.                                                                                                                                                                                                                                  |
| **잡코리아** (jobkorea.co.kr)                                     | ❌ skip           | Litigious history of pursuing scrapers via legal channels.                                                                                                                                                                                                                                                          |
| **게임잡** (gamejob.co.kr)                                        | ❌ skip           | Subsidiary of 잡코리아 — same litigious posture by inheritance.                                                                                                                                                                                                                                                     |
| **잡플래닛 채용** (jobplanet.co.kr/job_postings)                  | ❌ skip           | Cloudflare challenge wall AND operator with separate litigation history around review content. Strictly worse posture than 원티드.                                                                                                                                                                                  |
| **Indeed Korea** (kr.indeed.com)                                  | ❌ skip           | Cloudflare-blocked AND Indeed has a global track record of actively litigating and ToS-banning scrapers.                                                                                                                                                                                                            |
| **커리어리** (careerly.co.kr)                                     | ❌ skip           | NOT actually a job board (was floated as a 프로그래머스 replacement). It's a career-content SNS. "Job matching" is account-gated and curated; no public listings index to scrape.                                                                                                                                   |
| **LinkedIn Korea**                                                | ❌ skip           | hiQ v. LinkedIn precedent; aggressive enforcement; CAPTCHA-heavy.                                                                                                                                                                                                                                                   |
| **인크루트** (incruit.com)                                        | ❌ skip           | Low signal, defensive posture, dated UX.                                                                                                                                                                                                                                                                            |

## Per-portal notes

### 네이버 careers — shipped

Endpoint: `GET https://recruit.navercorp.com/rcrt/loadJobList.do`. JSON, no
auth, robots-permitted (the host's robots.txt 404s, RFC 9309 = unrestricted).
Single-phase: the listing carries every field we use, so `FetchDetail` is a
no-op. See `internal/scraper/naver/API_NOTES.md` for the field semantics and
the `entTypeCd` filter quirk (multi-value array filtering is silently
ignored; we filter `0010` 신입 + `0030` 무관 client-side).

Reality check: most days the 신입 universe is 0-3 postings. Naver hires 신입
primarily via 공채 cycles, which is parked separately in
`feature-ideas.md`. Until that integration lands, this scraper is most
useful as a "did 공채 just open?" early-warning signal.

### 잡알리오 (job.alio.go.kr) — next target

Government-run, public-information mandate (공공기관의 운영에 관한 법률).
Legacy JSP. Listings at `/recruit.do` (HTML), detail at
`/recruitView.do?idx={id}`. Many postings are "IT 일반" rather than
dev-specific, so the scoring matcher needs to handle "전산" /
"정보처리" tokens alongside dev stack tags. Worth a Step-0 spike to
size the work.

### 데모데이 (demoday.co.kr) — deferred (recon 2026-05-27)

The earlier read of this site was wrong on two points and the corrected
picture is what changed the verdict.

What the recon actually found, watching the listings page (`/recruits`)
load in a real browser:

- The listings HTML is a thin client shell (~30KB, no inline data). Data
  is fetched after page load from a **Supabase REST endpoint on a
  different host**:
  `https://xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits?...`
  The query returns the list of recruit IDs + lightweight metadata; a
  follow-up `?id=in.(…)` query fetches full records for the visible page.
- The same page also fires Next.js RSC requests at `/recruits/{id}?_rsc=…`
  for prefetch — but those go through `/_next/` semantically and the
  initial page bootstrap pulls from `/_next/` too.

Their robots.txt is **not** as friendly as previously documented. The
`User-Agent: *` block explicitly disallows both `/api/` and `/_next/`,
which is every server-side data path on the demoday.co.kr origin.

That leaves three options, all of which fail the project's polite-and-
small-binary thesis:

1. **Hit Supabase directly.** Technically out of scope of demoday's
   robots.txt (different host) and the anon key is embedded in the
   page, so anyone *could* query it. But the demoday operators clearly
   chose to disallow `/api/` on their domain — bypassing that intent by
   going around the front door is the kind of thing this project agreed
   not to do (see also 자소설닷컴, deferred for the same posture reasons).
2. **Run a headless browser inside the scraper.** That is the only way
   to obtain data through an allowed path — wait for the client to fetch
   Supabase, then read the rendered DOM. Adds a Chromium dependency,
   breaks the pure-Go + single-binary distribution story, and violates
   the "no CGO" constraint.
3. **Parse server-rendered HTML detail pages (`/recruits/{id}`).** Those
   URLs are allowed by robots, but they're also client shells — the
   detail data lands via the same Supabase fetch after JS runs. And
   there is no allowed surface that enumerates the IDs in the first
   place (the sitemap lists category pages only, not individual recruits).

Revisit if the project later adopts a Playwright-based scraping path
(would also unlock 카카오, 쿠팡, 원티드), or if 데모데이 publishes an
RSS / sitemap-of-recruits surface that doesn't depend on Supabase.

### 그룹바이 (groupby.kr) — deferred (recon 2026-05-27)

The HTTP-layer picture is friendly: a clean JSON API at
`https://api.groupby.kr/startup-positions` returns `{status, data:{total,
items}}` with no authentication required, paginated by `limit`+`offset`.
groupby.kr's robots.txt enumerates many named bot UAs (ClaudeBot,
GPTBot, ChatGPT-User, Perplexity, etc.) plus the wildcard `*`, all with
the same Allow `/` and Disallow `/api/` `/_next/` posture, and the API
host's robots.txt 404s = unrestricted.

The TLS-layer picture kills it: any non-browser HTTP client (curl, Go's
`net/http`) gets `HTTP/2 404` with an empty body and `server: nginx/
1.21.5`, regardless of how complete a browser-shaped header set we
send (Origin, Referer, full `sec-ch-ua-*` block, Accept, Accept-
Language, Accept-Encoding, all of it). The page works in real Chromium
because of TLS / JA3 / HTTP-fingerprint detection at the proxy layer.

That is the same blocker class as 원티드 / 쿠팡 / 잡플래닛 — would need
a headless browser or a TLS-fingerprint-spoofing client (utls/CycleTLS),
both of which break the pure-Go + single-binary distribution thesis. No
viable path without rethinking the scraping architecture.

Revisit if the project later adopts a Playwright-based scraping path
(would also unlock 데모데이, 카카오, 쿠팡, 원티드), or if 그룹바이 ever
relaxes the API-layer bot check.

### 프로그래머스 (career.programmers.co.kr) — defunct

Permanently retired by Grepp on 2025-04-28. The career subdomain no longer
resolves (NXDOMAIN). Official notice:
<https://programmers.co.kr/notices/11584>. Do not revisit.

### 카카오 careers — deferred

Full React SPA loaded from a 2.4MB bundle. All `/api/*` paths return 401
without a bootstrap auth token. None of the public endpoint patterns
(`/api/jobs`, `/api/jobOffer`, `/api/recruit`, `/api/v2/jobs`, etc.) work
unauthenticated. Plus the careers system is fragmented across 6+
subsidiary subdomains (careers.kakaoenterprise.com, career.kakaostyle.com,
kakaoent.com/career/recruit, career.kakaopaysec.com, recruit.kakaobank.com,
careers.kakao.com itself), each its own portal. If we ever do this, it's
6+ separate scrapers, not one.

### 쿠팡 careers (coupang.jobs) — deferred

Full Cloudflare challenge wall. Plain curl, fully-fingerprinted curl, both
get the "Attention Required! | Cloudflare" interstitial. Same blocker as
원티드 and 로켓펀치. Coupang's careers also flow through Workday for some
roles — could probe `coupang.wd3.myworkdayjobs.com` style URLs if needed.

### 삼성 careers — deferred

Two relevant Samsung portals exist and neither works for the Korean
new-grad use case:

- `sec.wd3.myworkdayjobs.com/Samsung_Careers` — Samsung Electronics public
  Workday tenant. POST `/wday/cxs/sec/Samsung_Careers/jobs` returns 493
  jobs across 26 countries. **South Korea is not in the country facet list
  at all.** This tenant carries Samsung's international hiring; KR jobs
  live in a separate authenticated portal.
- `samsungcareers.com/jobs` — returns `{"code":500,"status":999,"message":"Internal
server error"}` wrapped in HTTP 200. Either broken or auth-gated.

The KR-specific marketing page (`samsung.com/sec/about-us/careers/`) is
HTML only and links into the same auth-gated portal. Samsung's complexity
also extends to subsidiaries: Samsung SDS, Samsung C&T, Samsung Biologics
each have separate hiring systems.

### 로켓펀치 (rocketpunch.com) — deferred

Recon (2026-05-26) revealed a posture much less friendly than the older
community memory suggested:

- CloudFront-fronted with active bot detection. Plain curl gets 403; only
  a full Chrome-fidelity header set (User-Agent + Accept-Language +
  Sec-Fetch-\* + Accept-Encoding) gets through.
- robots.txt explicitly blocks `/*.json$` — which means the Next.js
  `_next/data/*.json` endpoints, the only clean scrape path, are off-limits.
- robots.txt also enumerates ~70 scraper user-agents and blanket-disallows
  them (including `Wget`).
- HTML pages technically render server-side with the data inlined, so
  scraping IS technically possible — but doing so would violate the spirit
  of the `/*.json$` rule.

If revisited, the legitimate path is a partner-API conversation, not
technical circumvention.

### 자소설닷컴 (jasoseol.com) — deferred (ToS)

The richest dataset of any portal evaluated: 11,582+ active recruit URLs
in the sitemap, mostly 대졸 신입/인턴, with a clean
`__NEXT_DATA__.pageProps.initialEmploymentCompany` object that gives us
everything (id, name, content, dates, recruit_type, employments[]).

But the ToS explicitly prohibits scraping: "자동화된 수단(예, 수집로봇,
스파이더, 스크래퍼)을 이용하여 사용자의 콘텐츠나 정보를 수집". Korean
case law (대법원 2021도1533) doesn't auto-criminalize ToS-only
violations, but combined with the Cloudflare layer this is meaningfully
more hostile than 점핏/랠릿. Treating jasoseol differently from
로켓펀치 would be inconsistent.

If the user explicitly accepts the trade-off, or contacts 앵커리어 about
a partner arrangement, this becomes the single highest-impact addition.

### 원티드 (wanted.co.kr) — deferred

Blocked behind Cloudflare bot detection — returns 403 even for
`/robots.txt` without a real-browser fingerprint. Options if we revisit:

- Headless browser (chromedp / Playwright) — kills the single-binary,
  no-CGO thesis. Likely a non-starter for v1.x.
- Paid residential-proxy + browser-impersonation library — adds ongoing
  cost, fragile.
- Direct contact with 원티드 for an API partner key — most legitimate
  path but requires a relationship.

### Direct company career pages — others

After the four big-tech recons (Naver shipped, Kakao/Coupang/Samsung
deferred), the remaining high-value candidates are mid-size dev-friendly
companies:

1. **토스** (toss.im) — careers.toss.im — high 신입 demand, JSON-friendly
2. **당근** (daangn.com) — about.daangn.com/jobs — fast hiring
3. **우아한형제들** (woowahan.com) — career.woowahan.com — 배민

Note: companies on 공채 (batch hiring) cycles publish less continuously
and may need closer integration with the 공채-calendar feature parked in
`feature-ideas.md` to be useful.

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

| Portal                           | Auth                                                                                                         |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| 점핏 / 랠릿 / 네이버             | None — public endpoints with browser-shaped headers                                                          |
| 워크넷                           | Per-user OpenAPI key (data.go.kr). **Disqualifying for v1.x** — see the onboarding-friction principle above. |
| 카카오 / 쿠팡 / 삼성 KR / 원티드 | Various forms of auth gating or bot defense. **Deferred.**                                                   |

**Never embed a credential in the binary.** It would be extractable from
the open-source repo in seconds, and would violate the issuing portal's
ToS for every install. This is also why per-user-key sources are deferred
entirely: the only architecturally correct path forces setup friction on
every user.

### Sweep behavior

The per-source `MAX(last_seen_at)` baseline in `SweepStalePostings`
(`internal/storage/postings.go`) means each source's freshness clock is
independent. A 점핏 scrape doesn't stale out 네이버 postings and vice
versa. Same goes for any future source — no change needed when adding one.

### Profile toggle

Each registered source gets a checkbox in the profile form's `공고 출처`
fieldset. Defaults to enabled; user can mute any source they find noisy.
Bookmarked postings are exempt from the toggle (explicit user signal wins).

### Source filter pills

The dashboard, archive, and bookmarks pages render a "전체 · 점핏 · 랠릿 ·
네이버" pill bar above the posting list when 2+ sources are visible. Filter
state is ephemeral (per page load) — switching pages or reloading resets
to "전체". Implementation in `web/source-filter.js`.
