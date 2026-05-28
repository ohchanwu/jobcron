# 데모데이 (demoday.co.kr) — scraper notes

Things I learned while exploring 데모데이's site on 2026-05-27. Written down here so the next person touching this doesn't have to re-derive them.

## Where the data actually lives

The 데모데이 site is a Next.js front end, but the job-posting data does NOT live on demoday.co.kr — it lives on a Supabase project hosted at `https://xypsryijdllrhfctnehy.supabase.co`. The in-page client makes two REST calls when you load `/recruits`:

1. A list query for lightweight metadata:
   ```
   GET https://xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits
     ?select=id,company_name,company_id,created_at,application_deadline
     &status=eq.published
     &is_active=eq.true
     &order=created_at.desc
     &limit=2000
   ```
   Supabase caps responses at 1000 rows by default — the request asks for 2000 and gets back 1000 with HTTP 206.
2. A bulk detail query for the visible page:
   ```
   GET https://xypsryijdllrhfctnehy.supabase.co/rest/v1/recruits
     ?select=*
     &id=in.(16966,16964,...)
   ```
   Returns the full record for each visible posting.

Both calls require two Supabase auth headers — `apikey` and `Authorization: Bearer <key>` — both set to the project's anonymous key. That key is publicly visible in the page bundle (`x-application-name: demoday-app`); it is not a secret.

The scraper picks the key in this order:

1. Env var `JOBSCRAPER_DEMODAY_ANON_KEY` if set (use this when 데모데이 rotates the key — paste the new value from a current page bundle).
2. `bakedInSupabaseAnonKey` constant in `demoday.go` otherwise.

If both end up stale, the next scrape returns HTTP 401 with a message that points at both rotation paths. That's the rotation contract for this scraper; there's no recovery beyond replacing the key.

## Robots.txt picture

- `demoday.co.kr/robots.txt` disallows `/api/`, `/_next/`, `/admin/`, `/login`, `/signup`, `/static/`, `/recruits/write` for `User-Agent: *`. None of those paths match what the Supabase scraper hits.
- `xypsryijdllrhfctnehy.supabase.co/robots.txt` returns 404, which RFC 9309 reads as unrestricted (the same pattern this project already uses for `jumpit-api.saramin.co.kr`).

`CheckAccess` therefore checks BOTH hosts: the Supabase host is the one whose disallows could actually block our requests, and the demoday host is the one our scraper "represents" from the user's mental model. If either turns hostile, the scraper aborts cleanly.

## Filtering down to 신입 IT/SWE

The `recruits` table has an `experience_level` column. The 2026-05-28 distribution across 1000 published+active rows shifted markedly from earlier samples:

| value | count |
| ----- | ----- |
| entry | 662   |
| 1-3   | 334   |
| any   | 4     |

The scraper uses `experience_level=in.(entry,1-3,any)`, so all three buckets enter the post-filter `keepsITSWE`:

1. Drop if `scraper.ParseExperienceYears(title, position)` returns `ok && minYears >= 4` — explicit 5년 이상 / 시니어 / 경력 5년 / 6년이상 patterns.
2. **Primary signal**: keep iff `position_tags[0] ∈ {개발, 게임 제작, 정보보호}`. Every observed record carries a `position_tags` array whose first element is the top-level job family. The category distribution on 2026-05-28 is heavily skewed toward non-SWE roles (마케팅·광고 255, 경영·비즈니스 196, 고객서비스·리테일 105, 영업 86, 디자인 79, HR 25, …) — the three IT categories together account for 167/1000 = 16.7% of the sample.
3. **Fallback (only when `position_tags` is empty/missing)**: keyword filter on title+position. Uses `itKeywordEN` (regex with word boundaries) and `itKeywordKO` (Korean substring set). Bare `개발` is deliberately not in `itKeywordKO` because it substring-matches non-dev compounds like 사업개발 (business development), 고객개발, 연구개발, 조직개발 — the explicit dev compounds (`프론트 개발`, `백엔드 개발`, `앱 개발`, etc.) are listed individually instead.

This is a 2026-05-28 rewrite. The previous filter ran only on the `any` bucket — that was correct when `any` carried the majority of rows, but the bucket distribution flipped and ~996/1000 rows now bypass it. The structured `position_tags[0]` signal is preferred over the keyword filter even where both work: "engineer" / "엔지니어" match mechanical, RF, aerospace, and semiconductor engineers (all sorted into `엔지니어링·설계` upstream), and "모바일" matches "모바일게임 로컬라이징 매니저".

Survivor classification of the 167 IT-category records on 2026-05-28: roughly 6 are non-SWE (game graphic designers, game planner, hotel-ops manager with "IT skills preferred", localization manager). That's ~96% clean SWE rate, well above the 90% target. Audit detail in `tmp/demoday_audit_2026-05-28.md`.

The `any`-bucket Posting carries `Newcomer=true`, `MinCareer=0`, `MaxCareer=anyBucketMaxYears` (99 — read as "no upper bound") so scoring treats it as new-grad-friendly.

## Record shape

`recruits` rows carry roughly 40 columns. The fields the scraper actually uses (and what they map to in `scraper.Posting`):

| Supabase column         | Posting field       | Notes                                                              |
| ----------------------- | ------------------- | ------------------------------------------------------------------ |
| `id` (int)              | `SourcePostingID`   | Stringified.                                                       |
| `title`                 | `Title`             | Free-form Korean / English.                                        |
| `position`              | (in Description)    | e.g. "Finance Manager"; folded into the composed description.      |
| `company_name`          | `Company`           |                                                                    |
| `location`              | `Location`          | Free-form address.                                                 |
| `experience_level`      | derived Newcomer    | `entry` → Newcomer=true; `1-3` → Newcomer=false, MinCareer=1, MaxCareer=3. |
| `application_deadline`  | `ClosedAt`          | ISO date or null. Null → AlwaysOpen.                              |
| `created_at`            | `PublishedAt`       | ISO timestamp.                                                     |
| `skill_tags` (string[]) | `StackTags`         | Empty for many records.                                            |
| `position_tags`         | `Tags` (category=position) |                                                              |
| `content` + `excerpt`   | `Description`       | Both contain HTML — stripped to plain text before storage.        |

Many records originate from 원티드 (`source_type: "wanted_api"`, `wanted_job_id: …`). Their `application_url` points at `https://www.wanted.co.kr/wd/{wanted_job_id}` — the scraper uses the demoday.co.kr URL (`/recruits/{id}`) as the user-facing link to keep the source attribution honest. Cross-portal dedup against 원티드 is parked until/unless 원티드 itself becomes scrapable.

## Rate limiting

One request per second, same pacing as every other scraper in this project. The active 신입 cohort is small enough (~28 rows) that a fresh scrape finishes in well under a minute.
