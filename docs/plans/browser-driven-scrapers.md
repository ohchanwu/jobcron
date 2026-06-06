# Plan: browser-driven / fingerprint-blocked scraper capability

**Status:** proposal for `/plan-eng-review` · **Date:** 2026-06-06 · **Author:** autonomous session

## TL;DR

The task was "plan a Playwright/browser-driven scraping path to unlock 원티드,
카카오, 쿠팡, 그룹바이." After recon, **the recommendation is: do not build the
browser capability for v1.x.** The premise that these four sources need a
headless browser is partly wrong, and where it's right, the sources either
cross the project's own ToS/operator-signal line or add little 신입-dev value:

- **쿠팡** needs **no browser at all** — its real listings are on the public,
  no-auth Greenhouse board API the app *already* scrapes (`coupang` token). The
  Cloudflare wall fronts only the marketing SPA at `coupang.jobs`. Verified
  2026-06-06: the board has 570 jobs / 276 Korea / **0 newcomer-marked dev
  roles** — a relevance skip, not an access problem.
- **그룹바이** has a **pure-Go path** (uTLS — single binary preserved, no
  browser). The existing scraper-list.md claim that "uTLS breaks the pure-Go
  single-binary thesis" is **factually wrong**. But its live board is
  experienced-skewed and overlaps 데모데이 / 랠릿 / 그리팅, and bypassing its
  TLS-fingerprint wall is an operator "no bots" signal.
- **원티드** genuinely needs a real browser (Cloudflare managed challenge; uTLS
  cannot pass it), **and** its Terms of Service (개인회원 약관 Art. 19 §7)
  explicitly prohibit both automated scraping and circumventing technical /
  CAPTCHA measures — the clearest prohibition of any source we've evaluated. Its
  legitimate path (official OpenAPI) is partner-key-gated, which fails the
  onboarding-friction principle.
- **카카오** continuous board is experienced-only; new-grad dev hiring is a
  once-a-year group 공채 fragmented across 6+ subsidiary single-page apps and
  syndicated to 공채 aggregators. High effort, ~zero continuous yield.

**Net:** the next 신입-dev value is in the no-auth Applicant Tracking System
(ATS) layer (그리팅 + multi-tenant Greenhouse), which is buildable today under
the pure-Go constraint and which captures most of what 원티드 would add. The
browser-gated tier is not worth the architectural escalation now. This doc
records the architecture *for if that ever changes*, the lightweight uTLS path
(decoupled from the browser question), the per-source verdicts, and two factual
corrections to scraper-list.md.

---

## 1. The premise to challenge

scraper-list.md groups 원티드 / 쿠팡 / 그룹바이 / 잡플래닛 as "same blocker —
needs a headless browser, which breaks the single-binary thesis," and notes a
Playwright path would "unlock several sources at once." Two things are wrong
with that framing:

1. **The four sources do not share one blocker.** They split into three
   technically distinct walls (below), with three different answers. Conflating
   them leads to the wrong build/decline calls.
2. **"Needs a browser" is conflated with "breaks the build."** Some of these
   are reachable in pure Go (uTLS); the real objections are *value* and
   *operator consent*, not the build pipeline.

## 2. Blocker taxonomy (the load-bearing distinction)

A request can be blocked at three independent layers. Beating one does **not**
beat the others.

| Layer | What it is | Tool that beats it | Single-binary safe? |
|---|---|---|---|
| **A — TLS/JA3 fingerprint** | A proxy hashes the TLS ClientHello (cipher list, extension order, curves) into a JA3/JA4 fingerprint and 404s anything that isn't a real browser, *before any app logic runs*. Go's `crypto/tls` emits a ClientHello no browser produces. | **uTLS** (`refraction-networking/utls`) — present a Chrome-shaped ClientHello. Pure Go, no JavaScript. | ✅ Yes (pure-Go, CGO-free) |
| **B — JavaScript challenge** | Cloudflare "Attention Required" / managed challenge runs an invisible JS proof-of-work + browser-fingerprint script after page load, then issues a `cf_clearance` cookie. | A **real headless browser** (chromedp/rod driving Chromium) + stealth + often residential proxies / a paid solver. uTLS cannot help (no JS engine), and Cloudflare re-fingerprints TLS every request anyway. | ❌ No (needs a managed Chromium) |
| **C — SPA + bootstrap-token auth** | A single-page app whose `/api/*` endpoints 401 until a token is minted by the app's own JS bootstrap; data is never in the served HTML. | Reverse-engineer the auth flow (net/http, sometimes possible) **or** a real browser. | ◐ Depends |

Mapping the four sources:

- **그룹바이** → Layer A only (the JSON API works in Chromium, 404s every
  non-browser client; the block is at the nginx/JA3 layer; no JS challenge).
- **원티드** → Layer B (Cloudflare managed challenge; 403s even `robots.txt`).
- **쿠팡** → none that matters: the listings live on Greenhouse, not behind the
  `coupang.jobs` Cloudflare SPA.
- **카카오** → Layer C (SPA + `/api/*` 401 + 6+ subsidiary subdomains).

## 3. If a browser path is ever adopted: the architecture

This section is for the contingency, not a recommendation to build now.

### Library choice

| Library | Pure-Go / CGO-free | How it gets Chromium | Breaks single-binary? |
|---|---|---|---|
| **chromedp** | ✅ Yes | Ships **none** — finds a system Chrome or connects to a remote CDP URL. | No — artifact stays one static file; runtime needs a Chrome somewhere. |
| **go-rod/rod** | ✅ Go layer | Auto-downloads a pinned Chromium (~150–200 MB) to `~/.cache/rod`, *or* uses a system one. | At artifact level no, but: **linux/arm64 (one of our two GoReleaser targets) is not in rod's standard download config** (only via the Playwright mirror at a different revision), and its default `leakless` helper extracts a guard executable that triggers antivirus false-positives. |
| **playwright-go** | Client is Go, but… | Drives a **bundled Node.js driver (~50 MB) + ~500 MB browsers over stdio**. | **Yes, worst fit** — trades a single static Go binary for a Node-dependent multi-artifact runtime. Reject. |

**Recommendation if ever built:** **chromedp**, behind a `//go:build browser`
tag, defaulting to **connect-to-an-external-Chrome via a CDP URL**
(`chromedp.NewRemoteAllocator` to a user-launched
`chrome --headless --remote-debugging-port=9222`, or `ExecAllocator` to find an
installed Chrome). This keeps the default release binary byte-for-byte
browser-free and ships zero browser bytes — the most honest fit for "single
self-contained binary." Reject playwright-go; park rod's auto-download (the
linux/arm64 gap + leakless surface are exactly the kind of quiet erosion of
"self-contained" the product thesis avoids).

### Distribution pattern trade-off

- `//go:build browser` tag → leanest default artifact, but browser sources are
  *compiled out* of the default binary (a runtime flag can't enable them; users
  need the browser build).
- No tag + opt-in runtime flag → one binary that can self-enable, at the cost of
  compiling chromedp in by default (small binary-size delta).

Pick deliberately. Given the calm/minimal thesis, the build-tag + external-CDP
combination keeps the 99% of users who never touch a browser source on a
pristine binary.

### Cross-compile / politeness / robots

- chromedp + the Go side cross-compile cleanly under `CGO_ENABLED=0` for
  linux/arm64 + darwin/arm64 (chromedp ships no platform-specific bytes).
- The browser path still uses the existing 1-req/s `waitForRateLimit` pacing.
- robots.txt + ToS are checked per source the same way 점핏/랠릿 were — the
  technical ability to drive a browser is not permission to (see §5).

**Verification gate before adopting:** a throwaway `//go:build browser` spike
that imports chromedp and confirms `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go
build ./...` stays clean and the binary-size delta is acceptable.

## 4. The lightweight path: uTLS for TLS-fingerprint sources (decoupled)

This is **independent of the browser question** and corrects a factual error in
scraper-list.md.

`refraction-networking/utls` is a fork of the standard `crypto/tls` that lets a
net/http-style client present a Chrome-identical TLS ClientHello. Its entire
dependency tree (brotli, klauspost/compress, `golang.org/x/{crypto,net,sys,
text}`) is **pure Go** — it compiles under `CGO_ENABLED=0` and cross-compiles
to linux/arm64 + darwin/arm64 with no C toolchain. **It does not break the
single-binary thesis.**

- Wiring cost: ~50–100 lines of `http.Transport.DialTLSContext` glue that wraps
  the TCP conn with `utls.UClient(..., utls.HelloChrome_Auto)`, runs the
  handshake, and routes to net/http (HTTP/1.1) or `x/net/http2` by negotiated
  ALPN. Reference impls exist (x04/cclient, bassosimone/utlstransport).
- **Sufficient for Layer A (그룹바이), useless for Layer B (원티드).** uTLS has
  no JS engine, and Cloudflare re-fingerprints TLS per request, so a managed
  challenge needs a real browser regardless.
- Costs: a maintenance treadmill (Chrome JA3 drifts; bump the `ClientHelloID` /
  uTLS version ~twice a year) and a slightly wider dependency surface. **Pin
  uTLS ≥ 1.8.2** (CVE-2026-26995 / CVE-2026-27017 affect 1.6.0–1.8.1).

So if 그룹바이 were ever desired, the honest blocker is **not** the build story
(uTLS keeps it pure-Go single-binary) — it's the operator-signal ethics (§5) and
the low value (§6).

## 5. Ethics / ToS — apply the project's own rule literally

The project already has a written decision rule:

- **Onboarding-friction principle** — no per-user account/key/credential setup.
- **"Not going around the front door"** — ships 데모데이 (no robots/ToS rule
  covers the host it hits), but declines 자소설닷컴 (ToS *explicitly* forbids
  자동화된 수집), and parked 배민 purely for a `robots.txt Disallow: /w1/**`.
- Skips 잡코리아 / Indeed KR / LinkedIn on active-enforcement history.

Applying it:

- **원티드 — out of bounds.** A Cloudflare managed challenge is an explicit
  operator "no bots" signal — a *stronger* stop than the robots.txt Disallow
  that parked 배민. 원티드's ToS (개인회원 약관 Art. 19 §7, eff. 2023-11-23) names
  the exact violations: "자동화된 수단(로봇/스파이더/스크래퍼)" **and**
  "Captcha를 외부 솔루션 등을 통해 우회하거나 무력화하는 행위" + IP rotation. A
  Cloudflare bypass would violate both clauses verbatim. This is the
  자소설닷컴 case, doubled.
- **그룹바이 — decline on operator-signal grounds, not build grounds.** A
  deliberate TLS-fingerprint bot wall is a softer-but-real "we don't want
  non-browser clients" signal. By the project's own 배민 precedent (parked for a
  *weaker* robots signal), the consistent call is to decline — after a proper
  groupby.kr ToS read, and on ethics, **not** on the (false) "uTLS breaks the
  build" reason currently in the doc.
- **쿠팡 — no ethics problem.** The Greenhouse board is the public, no-auth,
  robots-permitted, already-blessed route. Nothing to bypass.

## 6. Per-source verdict + value

Value = marginal 신입-dev volume *and* uniqueness vs what the app already reaches
(점핏 `career=0`, 랠릿, 데모데이, 그리팅, multi-tenant Greenhouse).

| Source | Wall | Reachable in pure Go? | 신입-dev value | Verdict |
|---|---|---|---|---|
| **원티드** | Cloudflare JS challenge | No (browser) | Highest *ceiling* of the four, but 신입 is the tail (mid-career platform) and its good employers overlap the ATS layer. | **Defer.** Not a Cloudflare fight; revisit only as a v2+ official-OpenAPI *partner* integration if the OpenAPI ever ungates. |
| **쿠팡** | (Cloudflare on marketing SPA only) | **Yes — Greenhouse, already built** | ~0 junior dev (verified: 570 jobs, 276 Korea, 0 newcomer-marked dev, 2026-06-06). | **Skip on relevance.** Same as the other senior-skewed Greenhouse tokens. No browser, no ethics issue. |
| **그룹바이** | TLS/JA3 | **Yes — uTLS** | Live board experienced-skewed; 신입 value is in seasonal intern programs (off-board); overlaps 데모데이/랠릿/그리팅. | **Decline.** Low value + operator anti-bot signal. (Build story is *not* the blocker — correct the doc.) |
| **카카오** | SPA + token, 6+ subdomains | No clean one | Continuous board ~0 신입; new-grad is a once-a-year group 공채, fragmented + syndicated to 공채 aggregators. | **Don't build.** Route any 대기업 신입 공채 desire to the parked 공채-calendar feature, not a per-subsidiary SPA scraper. |

## 7. Recommendation

1. **Do not build the browser-driven capability for v1.x.** No single source
   among the four justifies the architectural escalation, and 원티드 (the only
   high-ceiling one) is both ToS-prohibited to bypass and friction-gated via its
   legitimate API.
2. **Spend the scraper budget on the no-auth ATS layer instead** (그리팅
   multi-tenant + generalized Greenhouse token list) — buildable today under the
   pure-Go constraint, and it captures most of 원티드's worthwhile 신입-dev
   employers first-party and click-through-clean. *(Both shipped 2026-06-06.)*
3. **Record the architecture** (chromedp + `//go:build browser` + external-CDP)
   and the **uTLS path** so a future decision starts from facts, not the current
   errors.
4. **Reaffirm the ethics rule**: Cloudflare challenges and deliberate
   TLS-fingerprint walls are operator anti-bot signals; bypassing them is out of
   bounds, consistent with parking 배민 and declining 자소설닷컴.
5. **Route seasonal 대기업 신입 공채** (카카오/쿠팡) to a future 공채-calendar
   feature, not a scraper.

### What would change this decision

- 원티드 ungates its OpenAPI to a no-application read tier → 원티드 jumps in
  priority (both the Cloudflare and onboarding-friction objections fall away).
- A new TLS-fingerprint source appears that is *both* high 신입-dev value *and*
  robots/ToS-permissive → uTLS (not a browser) becomes worth the ~100 lines.
- The 공채-calendar feature ships → 카카오/쿠팡 seasonal 신입 signal arrives
  without any scraper.

## 8. Doc corrections to make (factual errors found)

1. **scraper-list.md 쿠팡 row** ("coupang.jobs — Cloudflare challenge wall, same
   blocker as 원티드") is stale: 쿠팡's listings are on the public Greenhouse
   `coupang` token (already evaluated, senior-skewed). Reconcile the row to
   point at Greenhouse, not Cloudflare.
2. **scraper-list.md 그룹바이 row** ("utls/CycleTLS break the pure-Go +
   single-binary distribution thesis") is wrong: uTLS is pure-Go and builds
   under `CGO_ENABLED=0`. The deferral should stand on operator-signal + value
   grounds, with the technical reasoning corrected.

## 9. Open questions for `/plan-eng-review`

- Is bypassing a deliberate TLS-fingerprint wall (uTLS, 그룹바이) categorically
  the same as bypassing a Cloudflare JS challenge (원티드), or a softer case
  worth a per-source ToS read? (This plan treats both as operator signals to
  respect, but they differ in strength.)
- Is the project ever willing to be a **partner API integrator** (원티드
  OpenAPI, 사람인 oapi, 잡코리아) despite the per-org key — i.e., is the
  onboarding-friction principle absolute, or relaxable for a high-value
  first-party feed behind an app-level (not per-user) key?
- Should the 공채-calendar feature be promoted from the parking lot, given it
  is the correct vehicle for the 카카오/쿠팡 seasonal 신입 signal that the
  browser path would otherwise (badly) chase?
