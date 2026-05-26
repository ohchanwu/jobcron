# 잡알리오 (job.alio.go.kr) — reverse-engineering notes

## Endpoint

```
GET https://job.alio.go.kr/recruit.do
```

Legacy JSP/JBCS httpd. **HTML response, not JSON.** robots.txt 404s; per
RFC 9309 that is unrestricted (matches the Jumpit API-host and Naver
recruit-host treatment).

The detail page at `/recruitview.do?idx={id}` (lowercase v) exists but we
do not fetch it — the listing carries every field the briefing uses, and
the detail page's added value (NCS code labels + PDF attachment) is not
worth a per-posting HTTP round trip.

## Filter parameters

| Param | Type | Notes |
|---|---|---|
| `detail_code` | repeated | NCS 표준직무 code; we use `R600020` (정보통신). |
| `career` | repeated | career-type code; we use `R2010` (신입) and `R2030` (신입+경력). |
| `pageNo` | int ≥ 1 | 1-indexed page number. |
| `pageSet` | int | per-page count (the form default is 10; we use 100). |

### NCS 직무 codes (from the form HTML)

The full table has 25 categories (R600001–R600025). For IT-신입 we
filter to `R600020` (정보통신) only. Adjacent categories that catch
some IT signal but mostly carry non-IT roles:

| Code | Korean | Use? |
|---|---|---|
| `R600019` | 전기·전자 | No — mostly hardware/EE |
| `R600020` | 정보통신 | **Yes** |
| `R600023` | 환경·에너지·안전 | No |
| `R600025` | 연구 | No — too broad |

If a user wants broader coverage, add codes to `ncsCategoryCodes` in
`alio.go`. Note: the more codes we OR together, the more non-IT roles
slip in, so the scoring step (user's keyword profile) carries more
weight on a wider net.

### Career codes

| Code | Korean |
|---|---|
| `R2010` | 신입 |
| `R2020` | 경력 |
| `R2030` | 신입+경력 |
| `R2040` | 외국인 전형 |

We OR `R2010` + `R2030`. `R2030` is critical — many public-sector
postings open to both 신입 and 경력 use this single code rather than
listing 신입 separately.

## Response shape

The response is a full HTML page rendered by the JSP. Each posting is
one `<tr>` in the listing table with cells in this fixed order:

1. row index (`<td>2147</td>`)
2. title + URL (`<td class="left"><a href="/recruitview.do?idx=300736" ...>TITLE</a></td>`)
3. company (`<td>학교법인한국폴리텍</td>`)
4. location (`<td> 경북 </td>`)
5. employment type (`<td> 무기계약직 </td>`)
6. posted date (`<td>2026.05.26</td>`)
7. closing date (`<td>26.06.02<br/>...<span>D-6</span></td>`)
8. status (`<td><span class="orange"> 진행중</span></td>`)

The parser pulls fields with anchored regexes against the `<a>` and the
sibling cells; non-matching rows are skipped silently. Whitespace inside
cells gets aggressively trimmed (the JSP emits tab-indented HTML).

## Posting field semantics

| Posting field | Source |
|---|---|
| `SourcePostingID` | `idx` from the `<a href>` |
| `URL` | `https://job.alio.go.kr/recruitview.do?idx={idx}` |
| `Title` | anchor text |
| `Company` | cell 3 |
| `Location` | cell 4 |
| `Newcomer` | always `true` (server-side filtered by `R2010`+`R2030`) |
| `CareerLevel` | always `"신입"` |
| `EducationName` | left blank (only on detail page) |
| `StackTags` | empty (legacy listing has no structured tech stack) |
| `Description` | synthesized from `title + company + location + employment-type` so FTS still has terms to match |
| `PublishedAt` | cell 6, format `YYYY.MM.DD` → KST midnight → UTC |
| `ClosedAt` | cell 7, format `YY.MM.DD` (2-digit year prefix!) → KST midnight → UTC |
| `AlwaysOpen` | always `false` (잡알리오 never has open-ended postings) |

### Closing-date year quirk

The listing cell shows `26.06.02` (2-digit year). We assume `20xx` for
any 2-digit year ≥ 50 maps to 19xx; that boundary is far enough out to
be safe for the lifetime of this scraper but is documented here in case
잡알리오 ever uses 4-digit format and the assumption needs revisiting.

## Quotas and etiquette

- No published rate limit. The scraper paces at 1 req/s to match the
  etiquette of the other scrapers; in practice a 잡알리오 scrape is
  exactly one HTTP request (the listing page with `pageSet=100`).
- Government site, public-information mandate (`공공기관의 운영에 관한
  법률`), no scraper-hostile language anywhere in ToS or robots.

## Known edge cases

- Many postings are "IT 일반" (정보통신 NCS code) rather than dev-specific
  (e.g. "전산직" administrative work, AMI operations). The keyword
  scoring stage handles refinement — make sure the user's profile has
  Korean dev keywords ("개발", "엔지니어") rather than English-only
  stack names if they want to filter further.
- The first row's `<td>` value is the row *index* on the page (e.g.
  2147), NOT the posting ID. Parser uses the `<a href idx=…>` for the
  ID, never the leading cell.
- Some titles include trailing inline `<span>` tags (status markers).
  We strip HTML inside the title.
