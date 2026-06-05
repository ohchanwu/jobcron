# Scraper List — v1.6+ Source Roadmap

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
| **네이버 careers** (recruit.navercorp.com)                        | ❌ removed        | Shipped 2026-05-25, removed 2026-05-27 after a click-through audit. The JSON listing API is fine, but the detail-page URLs the scraper stores **categorically don't deep-link for 신입 postings** — `recruit.navercorp.com` runs JavaScript on `view.do` that detects direct navigations and redirects to `/rcrt/list.do` (the generic listings page). Verified via playwright: a fresh 경력 annoId deep-links fine; every 신입 annoId (the only entType this scraper collects) redirects. Without a workable click destination there is no point shipping the source. See `.claude/sessions/2026-05-27-link-audit.md` for the recon trail.                                                                                              |
| **잡알리오** (job.alio.go.kr)                                     | ❌ removed        | Shipped 2026-05-26 then removed 2026-05-27. NCS R600020 (정보통신) filter does not actually deliver IT/dev roles in public-sector data — a 30-row audit found ~7% IT-adjacent (한전KDN AMI 작업원, 수도시설 운영, 마사회 인턴, etc.). Also surfaced an off-by-one parser bug: the live listing dropped the row-index TD that the unit test still includes, so `company` was getting written with the title. Fixing the parser wouldn't change the relevance verdict, so the source was unregistered. See `.claude/sessions/2026-05-27.md` for context.                                       |
| **데모데이** (demoday.co.kr)                                      | ✅ shipped        | Shipped 2026-05-27 via the embedded Supabase anon key (`xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits`). The robots.txt disallow at `/api/` is scoped to demoday.co.kr; Supabase is a different host whose robots.txt is unrestricted. Filter (rewritten 2026-05-28) keeps all three `experience_level` buckets and keys on `position_tags[0] ∈ {개발, 게임 제작, 정보보호}`, ~96% clean SWE; the `any`-bucket re-evaluation is resolved (`feature-ideas.md`).                                                                                                                                                                                                       |
| **그룹바이** (groupby.kr)                                         | ⏸ deferred        | Recon on 2026-05-27: the listings API (`api.groupby.kr/startup-positions`) returns clean JSON in Chromium but **404s every non-browser HTTP client** (curl, Go's `net/http`, even with the full set of browser headers including Origin/Referer/sec-ch-ua). nginx returns an empty 404 body — clear TLS-fingerprint / JA3-style bot detection. Same posture as 원티드 / 쿠팡 — out of reach without a headless browser, which this project explicitly avoids (pure-Go + single-binary distribution).                                                                                                                                                                                                                |
| **당근** (team.daangn.com)                                        | ✅ shipped        | Greenhouse public board API at `boards-api.greenhouse.io/v1/boards/daangn/jobs?content=true`. No auth. Single request returns ~42 jobs with full HTML body and rich metadata — `Engineer: yes/no` + `Prior Experience: 신입/경력/신입+경력` make filtering trivial. Recon + scraper landed 2026-05-27.                                                                                                                                                                                                                                                                |
| **Direct company pages — others** (Toss, 네이버페이)              | ✅ later phase    | One scraper each, shipped one per release. Toss: Greenhouse via api-public.toss.im, 236 jobs but ~0 신입 in titles (Toss hires for experienced engineers). 네이버페이: separate from existing 네이버 scraper, recon needed.                                                                                                                                                                                                                                                                                                                                |
| **배민** (career.woowahan.com)                                    | ⏸ deferred        | Recon 2026-05-27: heavy Vue SPA shell on career.woowahan.com (no SSR). The internal SPA fetches listings from `/w1/recruits` on the same host — BUT the host's `robots.txt` explicitly `Disallow: /w1/**` for `User-Agent: *`. Same posture as the original 데모데이 deferral. No alternate host or sitemap exposes the listings, so without going around the front door this scraper cannot be implemented. Defer until either (a) the project adopts a Playwright-based scraping path (which would also unlock 카카오 / 쿠팡 / 원티드 / 그룹바이), or (b) 배민 publishes a robots-permitted listings surface (sitemap-of-recruits, RSS, etc.). |
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

### 네이버 careers — removed 2026-05-27

Shipped 2026-05-25 (commit 19c0dfa) and removed 2026-05-27 after a
deeper click-through audit caught a fatal data-quality issue.

What the audit found (recon via playwright + curl):

- The listing JSON API at `/rcrt/loadJobList.do` returns clean rows
  with `jobDetailLink: "https://recruit.navercorp.com/rcrt/view.do?annoId=N"`.
  The scraper stored that link verbatim.
- Directly navigating to that link in a fresh browser tab serves a
  page that runs JS and **redirects to `/rcrt/list.do`** (the generic
  listings page) instead of rendering the posting. A `curl -L` follows
  the redirect, lands on `list.do` whose HTML contains the posting
  title from the listings array — which is exactly why the earlier
  curl-based audit gave false positives.
- Testing a fresh **경력** annoId pulled from today's listing: deep-link
  works, no redirect. Testing the live **신입** annoIds (the only kind
  this scraper collects): they redirect even when still in the live
  listing. The redirect is specifically gated on 신입 entType.
- Removing the redirect is impossible without controlling 네이버's
  JS — every URL the scraper stores will be effectively broken from
  the user's perspective.

So every 네이버 posting our app would have surfaced ends on the wrong
page when clicked. The scraper code is correct; the upstream careers
site is hostile to direct 신입 deep-links by design. Same posture as
잡알리오 — remove rather than ship a broken click destination.

Do not revisit unless either (a) 네이버 stops gating 신입 deep-links
on referer/cookie state, or (b) the project adopts a Playwright
scraping path that can also drive clicks (which is the same trigger
that would unlock 카카오 / 쿠팡 / 원티드 / 그룹바이 / 배민).

### 잡알리오 (job.alio.go.kr) — removed 2026-05-27

Shipped 2026-05-26 (commit ca7649e) and removed the next day. Two
reasons, both surfaced by a 30-row audit:

1. **The NCS R600020 (정보통신) filter does not actually deliver
   dev/SWE roles in public-sector data.** What it surfaces instead is
   field workers at 한전KDN AMI sites (metering), 수도시설 운영 (water
   utility operations), 한국마사회 인턴, 한국식품산업클러스터
   직원채용, 의료기기안전정보원 직원 — public-sector orgs that touch
   "telecom" or "IT" peripherally. The 30-row sample landed at ~7%
   even tangentially IT (주택도시보증공사 AX 전문가 + 한시적계약직
   전산직). Well past the 90%-not-IT threshold the audit task used.
2. **The parser had a silent off-by-one.** The live listing HTML
   dropped the row-index `<td>` cell that the unit test still
   included, shifting every field by one. As a result every row was
   stored with `company == title` and `location == actual company`.
   The big-fixture test only checked field non-emptiness, so it kept
   passing. Fixing the parser would not change the relevance verdict
   (the jobs still aren't IT), so the source was unregistered rather
   than repaired.

Do not revisit unless either (a) 잡알리오 publishes a finer-grained
NCS sub-code dedicated to dev/SWE roles, or (b) the project picks up
a 공공기관 cohort focus on its own merits and is willing to take the
noise.

### 데모데이 (demoday.co.kr) — shipped 2026-05-27

Briefly deferred during recon, then shipped the same day once the robots.txt
picture resolved in our favor. Full scraper notes (record shape, column
mapping, key-rotation contract) live in
`internal/scraper/demoday/API_NOTES.md`; the short version of *why it ships*:

The listings page (`/recruits`) is a Next.js shell (~30KB, no inline data) —
the posting data does NOT live on demoday.co.kr. The in-page client fetches
it from a **Supabase REST endpoint on a different host**:
`https://xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits`. A list query
pulls lightweight metadata (`status=eq.published&is_active=eq.true`, capped
at 1000 rows / HTTP 206); a follow-up `?id=in.(…)` query fetches the full
records. Both calls carry the project's **anonymous** Supabase key in the
`apikey` + `Authorization: Bearer` headers — that key is published in the
page bundle, is not a secret, and is exactly the read-only access Supabase's
anon role is designed to grant.

Why this is shippable and **not** "going around the front door" (the
distinction that keeps it on the right side of the 자소설닷컴 line):

- `demoday.co.kr/robots.txt` disallows `/api/`, `/_next/`, `/admin/`, etc.
  for `User-Agent: *` — but **none of those paths match what the scraper
  hits**. The scraper never touches a demoday.co.kr data path; it talks to
  the Supabase host.
- The actual data host, `xypsryijdllrhfctnehy.supabase.co`, serves a
  robots.txt that 404s, which RFC 9309 reads as unrestricted — the same
  two-host pattern this project already relies on for
  `jumpit-api.saramin.co.kr`.
- `CheckAccess` checks BOTH hosts and aborts cleanly if either turns
  hostile. The demoday disallow is scoped to its own origin; it does not
  reach across to the Supabase host, so respecting it does not require
  abstaining from the Supabase query.

That's the line that separates 데모데이 from 자소설닷컴: there the ToS
*explicitly* prohibits automated collection, so a friendly HTTP layer
doesn't matter; here no robots rule or ToS clause covers the path used.

Filtering to 신입 IT/SWE (rewritten 2026-05-28 after the bucket
distribution flipped — `any` went from ~720 rows to ~4): pull the `entry`,
`1-3`, and `any` `experience_level` buckets, drop explicit 4년+ roles, then
keep on `position_tags[0] ∈ {개발, 게임 제작, 정보보호}` (≈16.7% of rows)
with a title/position keyword fallback when tags are missing — ~96% clean
SWE rate. See API_NOTES.md for the full filter and audit detail.

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
(would also unlock 카카오, 쿠팡, 원티드), or if 그룹바이 ever
relaxes the API-layer bot check.

### 배민 (career.woowahan.com) — deferred (recon 2026-05-27)

career.woowahan.com is a Vue SPA — the landing returns a 22KB HTML
shell with ~200 prefetched chunk scripts and no inline data. The
single `<div id="app">` is hydrated client-side. Every path returns
the same shell (the SPA owns routing), so there is no HTML surface
to parse.

Inside the JS bundles (`chunk-common.3b770d52.js`) the recruitment
data fetcher is wired to:

```js
i = { recruit: "/w1/recruits", ... }
```

That endpoint lives on the same `career.woowahan.com` host. But the
host's `robots.txt`:

```
User-agent: *
Allow: /
Disallow: sign-in/**
Disallow: mypage/**
Disallow: /w1/**
```

explicitly disallows `/w1/**` for the wildcard user-agent. The only
known data endpoint is the only disallowed one. There is no second
host (Supabase-style) hosting the same data, and no sitemap, RSS,
or static enumeration of postings on the allowed surface.

Defer cleanly. Options the project might adopt later:

1. **Playwright path.** Run a headless browser, let the SPA hydrate,
   read the rendered DOM. Same trigger as 그룹바이 / 카카오 / 쿠팡 /
   원티드 — adopting this unlocks several sources at once.
2. **Wait for 배민 to publish a robots-allowed listings surface.**
   Sitemap-of-recruits, RSS feed, or moving listings out of `/w1/`
   would all qualify.

Do not write a scraper that hits `/w1/recruits` directly without
either of those conditions being met — that would be the kind of
"impolite scraping for one bullet point of coverage" this project
agreed not to do (see also 자소설닷컴, 로켓펀치).

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
