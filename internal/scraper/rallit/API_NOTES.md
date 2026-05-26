# 랠릿 OpenAPI — reverse-engineering notes

Companion to the design doc; lives next to the scraper so future maintainers
do not need to re-derive the field semantics.

## Endpoint

```
https://www.rallit.com/api/v1/position             # listing
https://www.rallit.com/api/v1/position/{id}        # detail
```

HTTPS, no auth, JSON. Response envelope is consistent across endpoints:

```json
{ "statusCode": "OK", "message": "...", "data": <payload>, "errorCode": "UNKNOWN_ERROR" }
```

`statusCode != "OK"` indicates an error; `data` then carries validation
details or is empty. We treat any non-OK as a request failure.

## Authentication

None. The listings path is open to anonymous traffic. Standard browser-shaped
User-Agent is enough.

## robots.txt posture

`https://www.rallit.com/robots.txt` allows `/api/*` paths (only `/applicants`,
`/apply`, `/auth`, `/my`, `/webview`, `/resume$`, `/resume-pdf`,
`/companies/788`, and `/sentry_sample_error` are disallowed). Our use is
within scope.

## Listing call

```
GET /api/v1/position
    ?pageNumber=1
    &pageSize=50
    &jobLevel=BEGINNER,INTERN,IRRELEVANT
```

| Param | Type | Notes |
|---|---|---|
| `pageNumber` | int ≥ 1 | required, 1-indexed (NOT 0-indexed) |
| `pageSize` | int ≥ 1 | per-page count cap (we use 50) |
| `jobLevel` | comma-separated enum | filter to 신입-friendly levels |

### Level enum (from live data sampling)

| Value | Meaning | Include for 신입 briefing? |
|---|---|---|
| `BEGINNER` | 신입 | ✅ yes |
| `INTERN` | 인턴 | ✅ yes |
| `IRRELEVANT` | 경력 무관 | ✅ yes (welcoming new grads) |
| `JUNIOR` | 1~3년차 | ❌ no |
| `MIDDLE` | 미들 | ❌ no |
| `SENIOR` | 시니어 | ❌ no |
| `TOP` | TOP / 리드 | ❌ no |

A posting may list MULTIPLE `jobLevels` (e.g. `["JUNIOR","MIDDLE"]`). The
server filter does "ANY match" — passing `BEGINNER` returns postings whose
`jobLevels` array CONTAINS `BEGINNER` (even if it also lists MIDDLE/SENIOR).
We accept that and let the scoring stage refine it; a posting open to both
신입 and 미들 is still relevant signal for the user.

### Filter param gotchas

- The plural `jobLevels` query param is silently ignored — it does not
  filter. Use the singular `jobLevel`.
- Repeating `jobLevel=BEGINNER&jobLevel=INTERN` returns a BAD_PARAMETER
  validation error. Use comma-separated values instead.
- Unknown query params are silently dropped, which makes typos hard to
  catch — discover params via deliberate validation-error probes (omit a
  required field and read the error response).

## Listing response (JSON)

```json
{
  "statusCode": "OK",
  "data": {
    "pageNumber": 1,
    "pageSize": 50,
    "totalCount": 131,
    "totalPage": 3,
    "items": [
      {
        "id": 4210,
        "title": "[토스플레이스 자회사/iShopCARE] Node.js Developer",
        "jobLevel": "IRRELEVANT",
        "jobLevels": ["MIDDLE"],
        "startedAt": "1970-01-01",
        "endedAt": "9999-12-31",
        "companyId": 1391,
        "companyName": "아이샵케어",
        "addressRegion": "GANGNAM",
        "status": { "code": "HIRING", "name": "모집 중" },
        "url": "https://www.rallit.com/positions/4210",
        "jobSkillKeywords": ["NestJS", "Node.js", "TypeScript"]
      }
    ]
  }
}
```

The `startedAt = "1970-01-01"` sentinel is the API's "we don't know when this
opened" placeholder; we drop it rather than persisting an Epoch published date.

The `endedAt = "9999-12-31"` sentinel means always-open — map to
`Posting.AlwaysOpen = true` and `Posting.ClosedAt = nil`.

## Detail call

```
GET /api/v1/position/{id}
```

Returns the same `data` shape as listing items but with many more fields. The
ones we actually consume:

| Field | Posting field | Notes |
|---|---|---|
| `title` | Title | |
| `companyName` | Company | |
| `addressMain` + `addressDetail` + `addressBuildingName` | Location | concatenated, trimmed |
| `jobLevel` | CareerLevel | the dominant level string |
| `jobSkillKeywords` | StackTags | array of stack names — already normalized |
| `description` + `responsibilities` + `basicQualifications` + `preferredQualifications` + `benefits` | Description | HTML, stripped of tags for FTS matching |
| `startedAt` / `endedAt` | PublishedAt / ClosedAt | YYYY-MM-DD KST → UTC |
| `isAlwaysHiring` | AlwaysOpen | explicit flag (parallel to the 9999-12-31 sentinel) |

The HTML strip is intentionally crude — replace tags with spaces, collapse
whitespace. We index the result for FTS, not display it; perfect rendering
is not required.

## Quotas and etiquette

- Not published. The site has no per-IP rate limit we can detect, but the
  scraper still paces at 1 req/s out of respect.
- robots.txt 200s and is permissive on `/api`. No re-check needed at
  scrape time beyond what `CheckAccess` already does.

## Edge cases known up front

- `startedAt = "1970-01-01"` is a sentinel for "unknown" — do not set
  `Posting.PublishedAt` for that value.
- `endedAt = "9999-12-31"` or `isAlwaysHiring = true` means always-open.
- HTML description can contain inline images and links via `<a>` and `<img>`
  tags; the tag-strip helper drops them entirely, keeping only the visible
  text.
- The same posting can list contradictory `jobLevel` (singular) vs
  `jobLevels` (plural) values. Trust `jobLevels` for our 신입 logic; use
  `jobLevel` only as a display hint.
