# 당근 (Karrot Market) — scraper notes

Recon performed 2026-05-27.

## Where the data lives

당근's careers page (`team.daangn.com/jobs`) is a Gatsby site, but the underlying ATS is **Greenhouse**. Greenhouse exposes a clean public Job Board API:

```
GET https://boards-api.greenhouse.io/v1/boards/daangn/jobs?content=true
```

- **No authentication required.** Plain `GET`, no headers beyond a User-Agent.
- **One call returns everything.** Pass `?content=true` and the response includes the full HTML body for each posting alongside metadata and location. No per-posting detail round trip is needed — `FetchDetail` is a no-op.
- **`absolute_url` points to about.daangn.com** with the `gh_jid` query param. The scraper uses that as the canonical Posting URL.

The 당근 careers domain (`team.daangn.com`) is a separate Gatsby surface that *also* renders the same jobs, but its `/page-data/jobs/page-data.json` only exposes a sparse `{corporate, employmentType}` shape — useless for our purposes. Always go through Greenhouse.

## Robots picture

- `team.daangn.com/robots.txt`: friendly — `Allow: /` with only `/dev-404-page/`, `/404/`, `/preview/`, `/completed/` disallowed.
- `boards-api.greenhouse.io/robots.txt`: only `/embed/` is disallowed. Our `/v1/boards/daangn/jobs` is unrestricted.

`CheckAccess` checks both hosts. No need to check `about.daangn.com` (the absolute_url host) — we don't request from it; we just hand the URL to the user as the click target.

## Metadata fields — direct 신입 IT filter

Every posting carries the same set of 16 metadata fields. The two that matter for filtering:

| field name        | possible values            | use                                                          |
| ----------------- | -------------------------- | ------------------------------------------------------------ |
| `Engineer`        | `true` / `false` (yes_no)  | Direct IT flag. 23 of 42 jobs are Engineer=true.             |
| `Prior Experience`| `신입` / `경력` / `신입/경력` | Direct seniority flag. 6 of 42 are 신입 or 신입/경력.       |

The scraper keeps a posting when `Engineer = true` AND the `Prior Experience` value contains "신입". On the 2026-05-27 snapshot that yields 4 postings — Software Engineer (Data) intern, two Frontend roles, and an ML role. Small but precise.

Other fields surfaced into Description for FTS:

| field                       | shape          | rendered into                              |
| --------------------------- | -------------- | ------------------------------------------ |
| `Corporate`                 | single_select  | Folded into Description; sometimes "당근알바" rather than the parent "당근". |
| `Employment Type`           | single_select  | `정규직` / `인턴`; mapped to a structured Tag with `Category: "employment_type"`. |
| `Alternative Civilian Service` | yes_no      | When `true`, adds a `병역특례 가능` welfare-category Tag — interacts with the existing `병특` dealbreaker matcher. |
| `Keywords`                  | short_text     | Free-text comma-separated tag list. Folded into Description. |
| `동료의 한마디`             | short_text     | One-line teammate quote. Folded into Description.   |
| `Tags`                      | short_text     | Free-text; folded into Description.                 |

## Filtering rationale

Why include `신입/경력` along with `신입`: postings tagged "신입/경력" explicitly admit 신입 applicants. Excluding them would miss real opportunities and contradict how the rest of the project handles the inverse (`Newcomer` flag) — we trust the source's stated seniority.

Why not include `경력` with Newcomer=false: those are pure-experienced postings the user (presumed 신입) won't get into. The experience-override parser added on 2026-05-26 wouldn't help either — `경력` is the source's own honest signal, not a misleading 무관-style tag.

## Record shape

| Greenhouse field          | Posting field    | Notes                                                              |
| ------------------------- | ---------------- | ------------------------------------------------------------------ |
| `id` (int)                | `SourcePostingID`| Stringified.                                                       |
| `title`                   | `Title`          |                                                                    |
| `absolute_url`            | `URL`            | `https://about.daangn.com?gh_jid={id}` — canonical for clicks.    |
| `location.name`           | `Location`       | Often just "SEOUL" — normalized to "서울" for the score chip.     |
| `company_name`            | (overridden)     | Greenhouse returns the parent corp; we use the `Corporate` metadata field instead because that captures the subsidiary (e.g. "당근알바"). |
| `first_published`         | `PublishedAt`    | ISO timestamp.                                                     |
| `application_deadline`    | `ClosedAt`       | Often null — null means AlwaysOpen.                               |
| `content`                 | `Description`    | HTML-encoded; stripped to plain text and stored.                  |
| metadata `Prior Experience` | derived Newcomer | `신입` or `신입/경력` → Newcomer=true.                           |
| metadata `Employment Type` | `Tags` (employment_type) |                                                          |
| metadata `Alternative Civilian Service = true` | `Tags` (welfare) "병역특례 가능" |                                    |

The full record JSON lands on `Posting.RawJSON` for forward compat.

## Rate limiting

One request per second, same pacing as every other scraper. The `?content=true` listing returns ~42 records in a single response; a fresh scrape is one round trip.
