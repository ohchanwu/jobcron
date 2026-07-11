# 그리팅 (Greeting / greetinghr.com) — notes

> Rename note (2026-07-11): This document records commands and paths used before
> the application was renamed from `job-scraper` to `jobcron`. Historical command
> output remains unchanged; current interfaces use `jobcron` and `JOBCRON_*`.

그리팅, by 두들린, is the leading Korean-native ATS. Each company runs a
server-rendered Next.js board at `{slug}.career.greetinghr.com`. Unlike the
Greenhouse boards, Greeting exposes a **structured 신입 signal**, so detection
is reliable rather than heuristic.

One `Source` (`"greeting"`) covers the whole curated tenant list — the
per-company name comes from each opening's `group.name`, and a single profile
toggle covers all tenants (24+ per-company toggles would bloat the UI).
`SourcePostingID` is `{slug}-{openingId}` for cross-tenant dedup.

## Where the data lives

Openings are inlined in the page's `__NEXT_DATA__` JSON — **not** the
`_next/data/{buildId}/...json` route, whose `buildId` rotates every deploy. We
parse `<script id="__NEXT_DATA__">` from the HTML:

```
props.pageProps.dehydratedState.queries[]  →  the entry whose queryKey == ["openings"]
  →  state.data[]   (array of openings)
```

The `["openings"]` query is found by decoding each `queryKey` as `[]string`:
the other queries carry object elements (`["publicCareer", "...", {...}]`) so
the `[]string` decode fails for them — exactly the discriminator we want.

### Opening shape

```
opening.openingId            → posting id (URL: {origin}/ko/o/{openingId})
opening.title                → title
opening.group.name           → company (e.g. "넛지헬스케어")
opening.openDate / dueDate   → PublishedAt / ClosedAt (null dueDate = 상시채용)
opening.openingJobPosition.openingJobPositions[]  → positions (an opening can bundle several):
   workspaceOccupation.occupation      → job family (개발 / 데이터 / 마케팅 / Tech/Product / …)
   workspaceJob.job                    → role (Backend / Flutter / 데이터엔지니어 / …)
   workspacePlace.{place,location,detailPlace}  → location
   jobPositionCareer.careerType        → NEW_COMER / NOT_MATTER / EXPERIENCED  ← the 신입 signal
   jobPositionCareer.careerFrom/To     → year range
   jobPositionEmployment.employmentType → INTERN_WORKER / FULL_TIME_WORKER / MILITARY_SERVICE_EXCEPTION(병특)
```

## Filtering (classify.go)

An opening is kept if **any** position passes all three gates:

1. **careerType ∈ {NEW_COMER, NOT_MATTER}** — skip EXPERIENCED. NOT_MATTER
   (경력무관) is 신입-eligible per the inclusive reading.
2. **Korea location** — these companies are multinational (캐시워크 has a
   Seattle office, etc.); `scraper.IsKoreaLocation` over the place string.
3. **dev role** — structured occupation first (개발 / 데이터 / *data* /
   *engineering* / *software* / *devops*), `scraper.HasDevKeyword` fallback for
   the many tenant-specific labels (테크, 기술, Tech/Product, DS, …). The
   structured allow matters because a clean job like "Flutter개발" (occupation
   개발) carries no token HasDevKeyword recognizes; the keyword fallback (which
   excludes bare 개발 and bare AI) keeps precision, dropping a "사업개발팀 영업"
   sales role and an "AI 전략 인턴" strategy role.

Plus an opening-level screen for non-jobs (`추천제도`, `지인 추천`, `인재등록`,
`인재풀`).

## Reachability gotchas

- **Landing path varies per tenant** — `/ko/home`, `/ko/career`, `/ko/corp`,
  `/ko/main`. We fetch `/` and follow the redirect; the job-URL origin is taken
  from the **final** post-redirect URL (`originOf`).
- **Custom domains** — 무신사 redirects to `musinsacareers.com`. The
  origin-from-final-URL approach handles these transparently. (무신사 is not
  shipped: ~135 openings, all 경력.)
- **Cloudflare** — fronts every board. As of 2026-06-06 it serves the real
  page to the project's honest `job-scraper/0.1` User-Agent (and even an empty
  UA). If it tightens to bot-wall non-browser clients, switch `userAgent` to a
  browser string (the recon flagged this as a soft guarantee).
- **robots.txt** — `User-agent: *` has `Allow: /` with only `/m/*`, `/a/*`, and
  `/o/*/apply` disallowed; the board (`/ko/home`, `/ko/o/{id}`) is permitted.
  AI-training bots (ClaudeBot, GPTBot, …) are `Disallow: /` in their own
  groups; our scraper honors the `*` group, not those.

## Curated tenant list (tenant.go)

Hand-maintained — 그리팅 has no public tenant directory and slugs aren't
guessable. Verified live 2026-06-06 (each resolves, exposes openings, yields
신입 dev). Shipped: cashwalk12 (캐시워크, biggest), estfamily (ESTgames/security),
realworld (RLWRLD AI), supercent, echomarketing, kimcaddie, blue-dot, kakaopay
(seasonal 신입크루). Dropped for zero yield / cost: 무신사 (custom domain, all
경력), 컬리 (IT-ops only), ezcaretech (referral pool only).

## Known limitation

The listing `__NEXT_DATA__` carries no JD body, so `Description` is composed
from the structured fields (title + occupation + job + employment) rather than
full text. The 병특 signal still flows through `employmentType =
MILITARY_SERVICE_EXCEPTION → 병역특례 가능` tag. If richer full-text matching is
needed later, implement `FetchDetail` to fetch `/ko/o/{id}` and parse its JD.
