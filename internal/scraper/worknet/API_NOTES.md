# 워크넷 OpenAPI — reverse-engineering notes

Companion to the design doc; lives next to the scraper so future maintainers
do not need to re-derive the field semantics.

## Endpoint

```
http://openapi.work.go.kr/opi/opi/opia/wantedApi.do
```

Plain HTTP (the public-data portal serves the spec over HTTPS but the API
endpoint itself is HTTP-only as of registration). We send our own
`User-Agent` and respect 1 req/s pacing.

Two call modes selected by `callTp`:

| `callTp` | Purpose | Returns |
|---|---|---|
| `L` | Listing | Pages of summary postings |
| `D` | Detail | One enriched posting given a `wantedAuthNo` |

This is the same two-phase shape as 점핏, so it fits the existing
`Scraper` interface cleanly: `FetchListing` calls with `callTp=L`,
`FetchDetail` calls with `callTp=D`.

## Authentication

API key is passed as the `authKey` query parameter — there is no header
auth. Keys are issued per app at <https://www.data.go.kr/data/3038225/openapi.do>
after registering an account. The key is a long URL-safe string; the
binary takes it via `--worknet-api-key` flag or `JOBCRON_WORKNET_KEY`
env var. **Never log the key**; redact it from any printed URL.

## Listing call

```
GET /opi/opi/opia/wantedApi.do
    ?authKey=KEY
    &callTp=L
    &returnType=XML
    &startPage=1
    &display=100
    &occupation=2211|2212|2213|2214  # IT codes, pipe-separated
    &minCareer=0
    &maxCareer=0                      # both 0 → 신입 only
```

| Param | Type | Notes |
|---|---|---|
| `authKey` | string | required |
| `callTp` | `L` or `D` | required |
| `returnType` | `XML` | only XML is documented; JSON not supported |
| `startPage` | int ≥ 1 | required |
| `display` | int ≤ 100 | per-page count cap |
| `occupation` | pipe-separated codes | KECO 직종코드. IT cluster is the 22xx series — see "Occupation codes" below |
| `region` | code | optional location filter; we leave unset and filter client-side |
| `empTp` | code | employment type |
| `minCareer` / `maxCareer` | int | years of experience; `0/0` for 신입 |
| `education` | code | optional |
| `keyword` | string | optional free-text |

The 신입-filter mechanism is `minCareer=0&maxCareer=0`. There is no
boolean "newcomer" flag in the public spec, so we filter by zero career
years and post-validate against `Posting.Newcomer = true`.

## Listing response (XML)

```xml
<wantedRoot>
  <total>NNN</total>
  <startPage>1</startPage>
  <display>100</display>
  <wanted>
    <wantedAuthNo>K2026...</wantedAuthNo>   <!-- posting ID -->
    <company>회사명</company>
    <busino>1234567890</busino>
    <indutyNm>업종</indutyNm>
    <title>채용 제목</title>
    <salTpNm>연봉제</salTpNm>
    <sal>3000~4000만원</sal>
    <region>서울 강남구</region>
    <empTpNm>정규직</empTpNm>
    <minEdubg>학사</minEdubg>
    <career>신입</career>
    <regDt>20260520</regDt>            <!-- YYYYMMDD -->
    <closeDt>20260620</closeDt>        <!-- YYYYMMDD; '99991231' when always-open -->
    <wantedInfoUrl>https://www.work.go.kr/wantedMain.do?wantedAuthNo=K2026...</wantedInfoUrl>
    <wantedMobileInfoUrl>...</wantedMobileInfoUrl>
    <jobsCd>2212</jobsCd>              <!-- 직종코드 -->
  </wanted>
  <wanted>...</wanted>
  ...
</wantedRoot>
```

Field shapes are derived from the public spec page and the DACON
analysis writeup; the live shape will be confirmed once the first
fixture is captured. **All shape assumptions should be re-validated
against the first real response — update this file when they drift.**

## Detail call

```
GET /opi/opi/opia/wantedApi.do
    ?authKey=KEY
    &callTp=D
    &returnType=XML
    &wantedAuthNo=K2026...
```

Returns the same `<wanted>` element plus long-form description, welfare
tags, contact info, and the full job-description text we feed into FTS.
Fields TBD until the first detail fixture is captured.

## Occupation codes (KECO 직종)

The IT cluster lives under the 22xx series. For v1.6 we hard-code these
four codes, which cover the bulk of 신입 software/engineering postings:

| Code | Korean | English |
|---|---|---|
| `2211` | 정보시스템 전문가 | Information systems |
| `2212` | 응용소프트웨어 개발자 | Application software developer |
| `2213` | 데이터 전문가 | Data professional |
| `2214` | 정보보안 전문가 | Information security |

The full common-code table is downloadable from
<https://www.data.go.kr/data/15037287/openapi.do>; we do not embed it
to avoid bloating the binary and to keep the codepath simple. If the
user wants broader coverage, add more codes to the package's
`occupationCodes` constant.

## Quotas and etiquette

- Daily quota per key is set by 공공데이터포털 at registration; default
  is in the low thousands. The scraper paces at 1 req/s like Jumpit.
- `robots.txt` for `openapi.work.go.kr` 404s, which RFC 9309 treats as
  unrestricted — we still hit it from `CheckAccess` to maintain the
  same shape as the Jumpit scraper.
- The 워크넷 ToS for the OpenAPI permits non-commercial reuse with
  attribution; we surface "Source: 워크넷" inline on each posting row
  to satisfy this without bloating the UI.

## Edge cases known up front

- `closeDt = 99991231` means "always open" — map to `Posting.AlwaysOpen = true`
  and `Posting.ClosedAt = nil`.
- `career` field is a Korean string ("신입", "1년", "1~3년"); we
  primarily rely on the `minCareer`/`maxCareer` filter we sent, but
  parse the string defensively for the `Posting.CareerLevel` display.
- `regDt` and `closeDt` arrive as YYYYMMDD without timezone; we read
  them as KST midnight and convert to UTC for storage, matching the
  Jumpit handling.
- Some postings have no `sal` (salary undisclosed) — leave the salary
  fields empty, do not error.
- Korean text in XML is UTF-8 but occasionally arrives with stray
  control characters; strip per the existing `cleanXMLText` helper
  (to be added).
