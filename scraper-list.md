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
| **카카오 careers** (careers.kakao.com)                            | ⏸ deferred        | Full SPA with all `/api/*` endpoints returning 401 — listing data needs a bootstrap auth token. Plus careers is fragmented across 6+ subsidiary subdomains (KakaoEnterprise, KakaoStyle, KakaoEnt, KakaoPaySec, KakaoBank…), each its own portal. Continuous board is experienced-only; new-grad dev is a once-a-year group 공채, syndicated to 공채 aggregators → route to a future 공채-calendar feature, not a per-subsidiary SPA scraper. See `docs/plans/browser-driven-scrapers.md`.                         |
| **쿠팡 careers** (coupang.jobs)                                   | ⏸ skip (low 신입)  | **Correction (2026-06-06):** the Cloudflare wall fronts only the `coupang.jobs` marketing SPA. 쿠팡's actual listings are on the public no-auth Greenhouse board API (`coupang` token) we already scrape — verified 570 jobs / 276 Korea / **0 newcomer-marked dev**. So this is a senior-skew relevance skip (same as the other Greenhouse senior tokens), NOT a browser/access problem. See `docs/plans/browser-driven-scrapers.md`.                |
| **삼성 careers** (sec.wd3.myworkdayjobs.com + samsungcareers.com) | ⏸ deferred        | Public Samsung Workday tenant carries 493 jobs across 26 countries but **zero in Korea** (KR jobs live in a separate authenticated portal). samsungcareers.com/jobs returns a 500 wrapped in 200. No accessible signal for Korean 신입 IT roles.                                                                    |
| **원티드** (wanted.co.kr)                                         | ⛔ decline (ToS)   | Cloudflare managed challenge (403 to robots.txt). Needs a real headless browser — and bypassing it would violate 원티드's ToS (개인회원 약관 Art. 19 §7) which explicitly forbids both 자동화된 스크래퍼 AND Captcha/기술적 조치 우회. Strongest prohibition of any candidate. Legitimate path = official OpenAPI (`openapi.wanted.jobs`), but partner-key-gated (fails onboarding-friction). 신입-dev is the tail (mid-career platform) and overlaps the ATS layer. See `docs/plans/browser-driven-scrapers.md`.                                              |
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
| **그리팅 / Greeting** (greetinghr.com, by 두들린)                 | ⏸ candidate (strong) | NEW Korean-native Applicant Tracking System (ATS), recon 2026-06-05. Per-tenant server-rendered Next.js boards at `{slug}.career.greetinghr.com`; job openings sit inline in the page's `__NEXT_DATA__` JSON (no headless browser needed). Structured `careerType` (`NEW_COMER`/`NOT_MATTER`/`EXPERIENCED`) gives a real 신입 signal — better than Greenhouse. ~24 Korean tenants verified live, 548 openings, ~59 신입-dev; strongest: `cashwalk12`(넛지헬스케어/캐시워크), `realworld`(RLWRLD), `estfamily`(이스트소프트), `supercent`. Click-through to a real posting verified. Build as ONE multi-tenant scraper over a curated slug list. See the ATS-platform note below. |
| **Greenhouse — Korean tenants** (boards-api.greenhouse.io)        | ◐ partial (당근 shipped) | No-auth public JSON board API — the exact model 당근 already ships. Recon 2026-06-05 verified NEW Korean boards to add: `krafton` (best — 6 junior/intern dev roles, all Korea), `moloco`, `sendbird`; senior-skewed (Korea volume, ~0 신입): `coupang`, `coupanginternal`, `seoulrobotics`; dead: `dunamu` (board 404), `appliedintuition` (public API gated). No structured 신입 field → title/description heuristic (counts are a lower bound). Click-through verified. Generalize the 당근 scraper into a token-list source. See the ATS-platform note below. |
| **Lever** (api.lever.co)                                          | ❌ thin            | No-auth JSON board API (`/v0/postings/{slug}?mode=json`), recon 2026-06-05. Korea-HQ startups essentially do **not** use Lever — 37+ native-company slugs all 404. Only `matchgroup` (Tinder/Azar Seoul) carries a real 신입 dev role (1 backend intern). Not worth a scraper for the 신입 thesis. |
| **Ashby** (api.ashbyhq.com)                                       | ❌ thin            | No-auth JSON board API (`/posting-api/job-board/{name}`), recon 2026-06-05. Real Korea boards exist (`furiosa-ai` 46 KR jobs, `twelve-labs`, `miso`, `hopae`) but ~0 junior/new-grad SWE — experienced-heavy. Skip for the 신입 thesis; revisit `furiosa-ai` only if a hardware/senior scope is ever added. |

## Per-portal notes

### ATS-platform layer (그리팅 / Greenhouse / Lever / Ashby) — recon 2026-06-05

This is a different axis from the company-by-company rows above. An Applicant
Tracking System (ATS) — the hiring software a company runs its careers page on
— is **not one source you scrape once**. It is a *platform* behind which each
company has its own board, reached by the same URL pattern with a different
identifier. The project already proved this twice without naming it: 당근 and
토스 are both on **Greenhouse**, each wired as a bespoke one-off.

The leverage: write **one scraper per ATS, parameterized by a curated list of
Korean company identifiers**. Adding the next Korean company on that ATS then
becomes a one-line config change, not a new scraper. A deep-research pass +
four discovery agents swept the four no-auth ATS platforms on 2026-06-05; every
candidate below was probed with a plain HTTP client (browser User-Agent, no
headless browser — matching the project's hard constraint), and the two live
candidates had a real posting **click-through-verified in a browser** (the
check that killed 네이버 and 잡알리오).

Net verdict: **two productive veins (Greeting, Greenhouse), two dead ones
(Lever, Ashby).**

**What counts as a 신입 dev role here.** The target audience is *anyone a
developer browsing GitHub for an entry-level job would consider* — not just
frontend/backend/fullstack/mobile software engineers (SWE). The filter
deliberately includes cybersecurity (정보보안 / DevSecOps), data engineers and
data analysts (데이터엔지니어 / 데이터분석), AI/ML engineers, DevOps / SRE / 인프라 /
클라우드, QA engineers, and embedded/firmware. The ~59 Greeting and ~9 Greenhouse
junior-dev counts below already use this broad reading — **do not narrow it to
"pure SWE."** This is slightly broader than 데모데이's `position_tags` filter (which
keeps `개발` / `게임 제작` / `정보보호`): data and DevOps/infra titles that 데모데이's
taxonomy may file under non-dev categories are in-scope for the ATS sources.

#### 그리팅 / Greeting (greetinghr.com) — the best NEW source found

Korea's leading native ATS (by 두들린), used by thousands of companies. Genuinely
new — not previously in this file.

- **Where the data lives.** Each tenant is a server-rendered Next.js page at
  `https://{slug}.career.greetinghr.com/`. The job openings are inline in the
  page's `<script id="__NEXT_DATA__">` JSON, at
  `props.pageProps.dehydratedState.queries` → the entry whose `queryKey` is
  `["openings"]` → `state.data` (an array). No headless browser needed — a plain
  HTTP GET + JSON parse gets everything. (There is also a
  `_next/data/{buildId}/ko/recruiting.json` route, but the `buildId` rotates on
  every deploy, so parsing `__NEXT_DATA__` from the HTML is the stable path.)
- **Real 신입 signal.** Each opening's `openingJobPosition.openingJobPositions[]`
  carries `jobPositionCareer.careerType` ∈ {`NEW_COMER` (신입), `NOT_MATTER`
  (경력무관/open to both), `EXPERIENCED` (경력)} and
  `jobPositionEmployment.employmentType` ∈ {`INTERN_WORKER`, `FULL_TIME_WORKER`,
  `MILITARY_SERVICE_EXCEPTION` (병역특례/병특), …}, plus `careerFrom`/`careerTo`
  and `workspacePlace.place` (address). This is a **structured** new-grad signal,
  unlike Greenhouse. Note the value is `NEW_COMER` with an underscore. Korean
  tenants lean on `NOT_MATTER` + title markers more than a strict `NEW_COMER`
  flag, so the project's existing inclusive reading applies (the demoday
  `any`-bucket convention — keep 경력무관/open-to-both as 신입-eligible).
- **Reachability / robots.** Cloudflare fronts it as a CDN but does **not**
  bot-wall a browser-User-Agent client (a default/empty-UA client may get 403 —
  send a normal UA, as the project already does for 점핏/랠릿). robots.txt is
  `Allow: /` with only `/apply`, `/m/*`, `/a/*` disallowed — the board path is
  permitted. This is a *softer* guarantee than a pure-API host; re-check if
  Cloudflare tightens.
- **Volume (2026-06-05).** ~24 Korean tenants verified, 548 total openings,
  ~59 신입-dev (inclusive count). Concentrated in a few strong tenants and a long
  tail of zeros. Best 신입/인턴 dev tenants, in order: `cashwalk12` (넛지헬스케어/
  캐시워크 — 17, full deck of 채용전환형 인턴: 백엔드/프론트/iOS/안드로이드/Flutter/
  데이터), `realworld` (RLWRLD robotics AI — 15), `estfamily` (이스트소프트 — 10),
  `supercent` (4), `ezcaretech` (2), `kimcaddie` (백엔드 신입~5년차), `blue-dot`
  (반도체 회로 설계 신입), `echomarketing` (백엔드 인턴/신입), `kbfintech` (Cloud
  Engineer 주니어). `kakaopay` (landing `/ko/main`) runs the 카카오그룹 신입크루 공채
  seasonally. Also live but mostly 경력: `musinsa`, `kurly`, `wadiz`, `gccompany`,
  `portone`, `megastudyedu`, `estfamily`. The custom-domain example is 올리브영
  (`career.oliveyoung.com`).
- **Click-through verified.** `cashwalk12/ko/o/78138` ("[캐시워크] 백엔드개발
  채용전환형 인턴") renders the exact posting in a browser, no redirect.
- **Why it's not trivial to ship.** Discovery is the hard part: tenant slugs are
  not guessable (Toss-on-Greeting is under a non-obvious slug; sendbird/lunit/etc.
  are NOT on Greeting), landing paths vary per tenant (`/ko/home`, `/ko/main`,
  `/ko/jobs`, `/ko/people`, `/ko/intro`, `/ko/corp`…), and some tenants use custom
  domains. A scraper needs a **curated, hand-maintained tenant list** (start from
  the ~24 above) plus per-tenant landing-path resolution (fetch `/` and follow the
  redirect). There is no public directory of all tenants. Per-tenant 신입 volume is
  modest; the aggregate is what makes it source-grade.
- **Verdict: strong candidate, build as one multi-tenant Greeting scraper.** The
  single best new source from this sweep.

#### Greenhouse — Korean tenants beyond 당근/토스

Same no-auth public JSON board API the project already ships for 당근:
`GET https://boards-api.greenhouse.io/v1/boards/{token}/jobs` → `{jobs:[…],
meta:{total}}`, no key, CloudFront-fronted but no bot challenge. (Toss is the
exception — it runs a custom Greenhouse host `api-public.toss.im`, not a standard
token.)

- **NEW verified Korean boards (2026-06-05):** `krafton` (63 jobs, 55 Korea, 6
  explicit junior/intern dev — the strongest; AI Research interns + 신입채용 track),
  `moloco` (67/20, Seoul ML/SWE interns), `sendbird` (17/9, Seoul AI engineer
  intern). High Korea volume but senior-skewed (~0 explicit junior dev): `coupang`
  (563/270), `coupanginternal` (336/136), `seoulrobotics` (10/10). Former/gated,
  now 404: `dunamu` (board unpublished), `appliedintuition` (uses Greenhouse but
  gates the public API / non-standard long token). `navervietnam` is Greenhouse
  but Vietnam-only — skip.
- **Caveat — no structured 신입 field.** Unlike Greeting (`careerType`) and 점핏
  (`career=0`), Greenhouse boards carry only `title` + `location` — junior
  detection is a title/description heuristic, so the ~9 junior-dev total across the
  live Korean boards is a **lower bound** (a plain "Frontend Engineer" open to 신입
  won't match a title filter). Same heuristic problem the 데모데이 scraper already
  solves.
- **Click-through verified.** `job-boards.greenhouse.io/krafton/jobs/8574562002`
  ("[AI Research Div.] Research Engineer … (2년 이상 / 인턴)", Seoul) renders the
  exact posting, no redirect.
- **Verdict: generalize the 당근 scraper into a multi-tenant Greenhouse source**
  parameterized by a token list (`daangn`, `krafton`, `moloco`, `sendbird`, + 토스
  via its custom host). Modest 신입 volume but zero-friction and already-proven.

#### Lever — evaluated, dead for Korean 신입 dev

No-auth JSON API (`api.lever.co/v0/postings/{slug}?mode=json`, robots `Allow: /`,
`Crawl-delay: 1` = the project's pacing). But **Korea-HQ startups don't use
Lever**: 37+ native-company slugs (Toss, 당근, Moloco, Sendbird, Hyperconnect,
Riiid, Lunit, Krafton, …) all 404. The boards with Korea roles are global firms
with Seoul offices (Match Group, Binance, Palantir, Xsolla, Insider, …), and
junior dev is near-absent — only `matchgroup` has a real entry-level role
("Software Engineer Intern, Backend (Tinder Seoul)"). One intern role is not worth
a scraper. Skip; the productive veins for Korean 신입 dev are the native boards
(랠릿, 그리팅) and Greenhouse, not the global ATSes.

#### Ashby — evaluated, dead for Korean 신입 dev

No-auth JSON API (`api.ashbyhq.com/posting-api/job-board/{name}`). Adoption among
Korea-HQ companies is thin, and what exists is experienced-heavy: `furiosa-ai`
(46 Korea jobs, but its one 신입-tagged role is semiconductor mass-production
evaluation, not software), `twelve-labs` (24, all Staff/Senior ML), `miso` (22,
ops-heavy), `hopae` (7, mid-level). Strict filter: **0 junior/new-grad SWE across
all of them.** Most Korea-located Ashby roles are on foreign boards (OpenAI,
Sierra, Speak) and senior. Skip for the 신입 thesis; `furiosa-ai` is the only board
worth remembering, and only if a hardware/senior scope is ever added.

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

This is a TLS/JA3-fingerprint block — a DIFFERENT class from 원티드's
Cloudflare *JavaScript* challenge (uTLS beats the former, not the latter).

**Correction (2026-06-06):** the earlier claim that "utls/CycleTLS break the
pure-Go + single-binary distribution thesis" is **wrong**. `refraction-
networking/utls` is a pure-Go fork of `crypto/tls` (whole dep tree is pure Go);
it compiles under `CGO_ENABLED=0` and cross-compiles to linux/arm64 + darwin/
arm64 — the single static binary is fully preserved. So the build story is NOT
the blocker. The honest reasons to keep 그룹바이 deferred are (a) it deliberately
circumvents an operator-deployed TLS bot wall — an anti-bot signal that, by our
own 배민 precedent (parked for a weaker robots signal), warrants declining, and
(b) low value: the live `/positions` board is experienced-skewed and overlaps
데모데이 / 랠릿 / 그리팅. uTLS would be ~100 lines of `DialTLSContext` glue (pin
≥ 1.8.2 for CVE-2026-26995/27017). Full analysis: `docs/plans/browser-driven-
scrapers.md`.

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
