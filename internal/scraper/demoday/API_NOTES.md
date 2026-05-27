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

Both calls require two Supabase auth headers — `apikey` and `Authorization: Bearer <key>` — both set to the project's anonymous key. That key is publicly visible in the page bundle (`x-application-name: demoday-app`); it is not a secret. The scraper embeds it as a constant.

## Robots.txt picture

- `demoday.co.kr/robots.txt` disallows `/api/`, `/_next/`, `/admin/`, `/login`, `/signup`, `/static/`, `/recruits/write` for `User-Agent: *`. None of those paths match what the Supabase scraper hits.
- `xypsryijdllrhfctnehy.supabase.co/robots.txt` returns 404, which RFC 9309 reads as unrestricted (the same pattern this project already uses for `jumpit-api.saramin.co.kr`).

`CheckAccess` therefore checks BOTH hosts: the Supabase host is the one whose disallows could actually block our requests, and the demoday host is the one our scraper "represents" from the user's mental model. If either turns hostile, the scraper aborts cleanly.

## Filtering down to 신입

The `recruits` table has an `experience_level` column. Distinct values observed on 2026-05-27 across the first 1000 rows:

| value         | count |
| ------------- | ----- |
| `any`         | 726   |
| `5+`          | 208   |
| `3-5`         | 37    |
| `entry`       | 22    |
| `1-3`         | 6     |
| `경력 2~4년` | 1     |

The scraper uses `experience_level=in.(entry,1-3)` to keep the dashboard quiet. `any` is intentionally excluded — it is a real signal in the data (725 of those rows are "no preference" postings, many of which are 신입-friendly), but including it triples the daily-briefing row count for one source, and the experience-override parser introduced in the 2026-05-26 session already catches the inverse case (a posting tagged 신입 / `any` whose title actually demands 5+ years). If the user later wants broader coverage we can flip the constant to `in.(entry,1-3,any)`.

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
