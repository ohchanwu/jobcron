# Naver Careers API — reverse-engineering notes

## Endpoint

```
GET https://recruit.navercorp.com/rcrt/loadJobList.do
```

JSON, no auth. The same endpoint serves the entire open-posting list across
all Naver-group brands (NAVER, NAVER LABS, NAVER WEBTOON, NAVER CLOUD,
NAVER FINANCIAL, NAVER I&S, etc.). Volume is small enough (typically under
30 open postings total) that a single fetch covers everything.

robots.txt at this host 404s, which RFC 9309 reads as unrestricted — same
treatment as `jumpit-api.saramin.co.kr` in the 점핏 scraper.

## Parameters (from move_form in /rcrt/list.do)

The page's filter form serializes these into the GET request:

- `subJobCdArr` — sub-job filter (array)
- `sysCompanyCdArr` — company filter (array; "KR" = NAVER itself)
- `empTypeCdArr` — employment type filter
- `entTypeCdArr` — career-type filter (`0010`=신입, `0020`=경력, `0030`=무관)
- `workAreaCdArr` — work area filter
- `firstIndex` — paging offset (0-indexed)

**Filter gotcha**: `entTypeCdArr=0010` alone works (returns only 신입). But
combining multiple codes — whether comma-separated (`0010,0030`) or by
repeating the param — silently returns the full unfiltered list. The Naver
backend apparently doesn't honor multi-value entTypeCdArr the way other
arr-style params probably do.

**Pragmatic workaround**: omit the filter entirely (return all ~25-30
postings) and apply the 신입+무관 filter client-side in Go. The volume is
trivial; we keep the round-trip count to one.

## Response shape

```json
{
  "result": "Y",
  "totalSize": 22,
  "list": [
    {
      "annoId": 30004895,
      "annoSubject": "[NAVER] 네이버 사내 부속의원 간호사 (계약)",
      "sysCompanyCdNm": "NAVER",
      "entTypeCd": "0020",
      "entTypeCdNm": "경력",
      "classCdNm": "Corporate",
      "subJobCdNm": "Health Care",
      "empTypeCdNm": "계약",
      "staYmd": "20260520",
      "endYmd": "20260602",
      "staYmdTime": "2026.05.20 10:00:00",
      "endYmdTime": "2026.06.02 10:00:00",
      "stateCd": "0040",
      "stateCdNm": "채용진행중(게시/마감후미노출)",
      "jobDetailLink": "https://recruit.navercorp.com/rcrt/view.do?annoId=30004895"
    }
  ]
}
```

Field semantics that matter:

| Field | Posting target | Notes |
|---|---|---|
| `annoId` | SourcePostingID | int; format as string |
| `annoSubject` | Title | usually prefixed with `[BrandName]` |
| `sysCompanyCdNm` | Company | "NAVER" / "네이버랩스" / "네이버웹툰" / "NAVER Cloud" / "NAVER Financial" / etc. |
| `classCdNm` + `subJobCdNm` | StackTags / Description fragment | English labels like "Tech / Frontend Development" |
| `entTypeCdNm` | CareerLevel | Korean string |
| `entTypeCd` | (filter) | `0010` 신입 / `0020` 경력 / `0030` 무관 |
| `empTypeCdNm` | (synthesized description) | "정규직" / "계약" / "인턴" |
| `staYmd` / `endYmd` | PublishedAt / ClosedAt | YYYYMMDD KST |
| `jobDetailLink` | URL | absolute, points to the HTML detail page |

## Detail endpoint

There is no JSON detail endpoint. `/rcrt/view.do?annoId=…` serves HTML.
The HTML embeds a `<script type="application/ld+json">` block with
schema.org JobPosting fields including the description; if we ever need
the long-form description we should parse that JSON-LD rather than
scrape the rendered DOM.

For v1.1 we leave Description empty and synthesize a short
keyword-friendly stand-in from `classCdNm + subJobCdNm + empTypeCdNm` so
FTS still has something to match.

## Quotas and etiquette

- No published rate limit. The scraper paces at 1 req/s to match the
  etiquette of the 점핏/랠릿 scrapers; in practice a Naver scrape is
  exactly one HTTP request (the listing).
- POST to the listing endpoint returns an alert script ("접근권한이
  없습니다") — use GET only.

## Edge cases known up front

- Most days the 신입 universe is empty or near-empty (0-3 postings).
  Naver hires 신입 primarily via 공채 cycles, which is parked separately
  in `feature-ideas.md`. Until that integration lands, this scraper is
  most useful as a "did 공채 just open?" early-warning signal.
- `entTypeCd=0010` AND `entTypeCd=0030` both count as 신입-friendly for
  scoring purposes — 무관 (any-experience) listings welcome new grads
  even when they aren't labeled 신입.
- `staYmd`/`endYmd` arrive as `YYYYMMDD` (no separator). KST midnight,
  converted to UTC at parse time.
