# Feature Ideas — Parked

_[한국어로 보기 🇰🇷](feature-ideas.ko.md)_

Things we discussed but explicitly cut from v1 to protect scope. Each entry should answer: what is it, why we want it, why it's not v1, and what triggers building it.

---

## Resume upload → auto-extract profile

**What.** Drop a resume (PDF / Markdown / plain text) into the profile-setup UI. App parses it, extracts stack tags, years of experience, education, location, etc. Shows the extracted profile in an editable preview. User confirms or edits, then saves.

**Why we want it.** Magical first-run experience. Lowers the cognitive cost of profile setup from "fill out a 20-field form" to "drop your resume here." Big "whoa" moment for new users and demo viewers.

**Why not v1.** Adds a whole new problem domain (PDF parsing, layout extraction, multilingual NER for Korean + English résumés). Parsing failures during first-run UX are catastrophic — the first impression breaks. Better to ship the structured form, see how people actually fill it, then build the parser against real shapes.

**Build trigger.** v1 has 50+ users AND structured-form completion rate is below ~70% (i.e. the form is friction). Then resume upload becomes the obvious unlock.

---

## AI / ML scoring layer

**Status (2026-06-06).** The *hosted* half of this shipped as the **v2.0 BYOK-AI line** — an LLM reads each posting and returns an evidence-cited Stage-2 score delta, layered on the keyword breakdown (additive, never a replacement — exactly as the build trigger below prescribed). OpenAI was offered, then **removed**: the rate-limit risk flagged in "Why not v1" (1) materialized — its free/entry tier couldn't sustain the re-rate workload. **Anthropic is the one provider.** The still-open direction is the **local / built-in model** (gguf / ONNX) — no rate limits, fine-tunable — now parked as a design task in `task-list.md`.

**What.** Replace (or augment) the current keyword-and-weight scorer with a semantic model — embeddings + cosine similarity between profile and posting, or an LLM that reads each posting and outputs a fit score with a short rationale. Could be local (gguf / ONNX) or hosted (Anthropic / Together / …).

**Why we want it.** Current matching is token-exact: "개발" doesn't match "개발자", "야근 없음" doesn't get distinguished from "야근", particle-attached forms like "야근이" miss "야근". A semantic model would close most of those gaps and pick up nuance the keyword matcher cannot (e.g. "성장하고 싶은 신입 환영" ≈ "신입 친화적").

**Why not v1.** Three reasons. (1) Breaks the "single-binary, no network at runtime, no API keys" distribution thesis — a hosted LLM means cost handling, key storage, rate limits, offline degradation. (2) Current scoring is intentionally legible — "why did this score 60?" has an obvious answer in the breakdown chips. Semantic scores are opaque ("the model said so"), which fights the calm-and-trustworthy product thesis. (3) You can't tell if an ML scorer is *actually* better without a labelled ground-truth set, and that set doesn't exist until v1 has been running long enough across multiple sources to accumulate "yes/no, was this a good match" signal.

**Build trigger.** v1 has been used for at least 2-3 months across multiple portals AND the user has explicit examples of "the keyword matcher should have flagged this but didn't" / "it scored this high but it's actually irrelevant." Then build a labelled eval set from real usage, then layer the model on top — never as a replacement, always as an additional signal alongside the keyword breakdown.

---

## Cross-model AI-extraction reuse for token-efficient Stage-2 scoring

**What.** Let the profile-aware Stage-2 scorer inspect the cached, posting-only
`ai_extractions` facts and evidence before sending the full job description. When
the structured extraction is sufficient, Stage 2 can use that compact evidence;
otherwise it falls back to the full posting text. Haiku, GPT mini, or another
validated model may reuse the same extraction rather than repeating identical
public-content analysis.

**Why we want it.** The extraction is independent of any user's profile and can
be reused across users. Treating it as a compact evidence layer could reduce paid
input tokens, latency, and duplicate model work while keeping the final score and
delta user-specific.

**Why not now.** The current `ai_version` combines provider, model, and one shared
prompt-template version for both extraction and scoring. A cache hit therefore
proves only that the exact current model/prompt combination produced the row. The
problem is not that two models can never share facts; it is that schema meaning,
prompt behavior, and evidence quality can change independently. Safe cross-model
reuse first needs a provider-neutral extraction contract, a separate extraction
schema/prompt version, evidence validation against the current posting, and an
evaluation showing that the compact path does not degrade Stage-2 decisions.

**Build trigger.** Stage-2 input cost or latency becomes material, and an eval set
shows that using normalized extractions first preserves score and evidence quality.
Then split extraction compatibility from provider/model identity and measure the
compact-path hit rate before making it the default.

---

## Self-healing Ollama runner restart

**What.** When the local-model provider is using Ollama and an inference wedges the runner, the app would detect the stuck state, stop the bad Ollama runner process, restart or reload it, and continue the overnight batch without the user waking up to press anything.

**Why we want it.** The local-model batch exists so the user can rate the whole `/archive` / 전체 공고 set unattended. The 2026-06-09 spike showed a real failure mode: one hung inference left Ollama alive but not generating, and later model calls silently blocked behind it. A health check that only asks "is Ollama running?" misses that. Full self-healing would make the overnight run genuinely hands-off.

**Why not v1.** Process management is a bigger responsibility than the app currently takes on. It means finding the right Ollama runner process, killing it safely, restarting or reloading the model, and handling platform-specific edge cases. v1 should do the smaller, safer version: run a real generation/liveness probe, retry boundedly, then stop with clear restart instructions. Finished postings are already cached, so pressing again resumes without repeating completed AI calls.

**Build trigger.** Build this if wedged-runner failures recur in normal Qwen2.5-7B use, not just during model-swapping experiments, or if "restart Ollama and press again" proves too disruptive for the intended overnight workflow.

---

## Hybrid: structured form + free-form keyword overrides

**What.** Keep the structured profile form as the primary input, but add two free-form fields underneath: "반드시 포함해야 할 키워드" (must-include) and "절대 포함되면 안 되는 키워드" (dealbreakers). Real Korean job preferences have long-tail signals that don't fit any structured field: 병특 transferable, 자유 출퇴근, 노가다 거부, 적대적 완전 재택, 외국계, 스톡옵션, 외국어 사용 환경, etc.

**Why we want it.** The structured form alone can't capture every signal. Free-form keywords fill the long tail without bloating the form schema. Strict include/exclude semantics keep the scoring engine simple and predictable.

**Why not v1.** v1 is testing whether structured scoring against a fixed schema is *enough*. Adding free-form overlays before we know the baseline answer would muddy the signal. Also: dealbreaker keywords on long-form posting text need careful matching (token boundaries, negation handling) — that's its own small project.

**Build trigger.** v1 users report that the structured profile misses important signals. First implementation should be additive only — keyword overrides modify the score, they don't replace any structured field.

---

## Additional scrapers (원티드, Programmers, Saramin, JobKorea, direct company pages)

**Update (2026-06-06).** Multi-portal scraping has since shipped: 점핏 · 랠릿 · 데모데이 · 그리팅 + the Greenhouse company boards (당근 · 크래프톤 · 몰로코 · 센드버드), plus optional 워크넷. The browser-gated portals (원티드 / 카카오 / 쿠팡 / 그룹바이) were evaluated and **decided-against** — 원티드 on ToS, the others on low 신입-dev value / operator anti-bot signals; 쿠팡's data is already on the public Greenhouse `coupang` token (~0 신입). See the durable [decision](../superpowers/decisions/260606-no-browser-driven-scraping.md) and the detailed [research and contingency architecture](../research/2026-06-06-browser-driven-scrapers.md). Remaining unscraped sources (Programmers, Saramin/JobKorea official APIs) are partner-key-gated → conflict with the onboarding-friction principle.

**What.** Implement the `Scraper` interface for additional sources, register them, let the user pick which to enable in the profile.

**Why we want it.** v1's biggest weakness vs the user's stated problem: 점핏 alone doesn't cover Naver/Kakao/Coupang/Toss/배민/당근 career pages, and doesn't cover non-dev-focused portals where 신입 IT roles sometimes appear.

**Why not v1.** Each scraper is a separate parser, with its own DOM/API shape, its own rate-limit etiquette, and its own anti-bot considerations. 원티드 is CDN-blocked (returns 403 to robots.txt) — likely requires headless browser or proxy infrastructure. Direct company pages change without warning. The maintenance cost compounds linearly with each source.

**Build trigger.** v1 of 점핏-only is solid and the architecture has held up. The user wants more coverage AND has the patience to maintain the new scrapers. Add one new source per release; never bundle a "scrape everything" mega-release.

---

## 공채 (batch hiring) calendar integration

**What.** Track 상반기/하반기 공채 schedules for major Korean companies (Naver, Kakao, Samsung, LG, etc.). Surface "3 days until 카카오 신입 공채 마감" on the daily briefing.

**Why we want it.** 공채 deadlines are uniquely important for 신입 — missing one means waiting 6 months. No existing tool integrates 공채 calendars with active scraping. Would be a clear differentiator for the 신입 cohort.

**Why not v1.** Requires a curated 공채 schedule data source (manual? scraped from korecruIT? scraped from jojoldu/junior-recruit-scheduler?). Curation overhead + integration UI design + the fact that 점핏 doesn't carry most 공채 listings → significant scope expansion.

**Build trigger.** v1 is shipped and used. Community contribution opens up — someone with knowledge of 공채 cycles wants to contribute a data source.

---

## Background scheduler / "set it and forget it" mode

**What.** Optional daemon mode where the binary keeps running, scrapes on a schedule (e.g. every morning at 8 AM), and shows the daily briefing the moment the user opens the app.

**Why we want it.** "Calm morning ritual" is even more powerful when the briefing is already prepared. Removes the 30-second scrape wait from the user's morning.

**Why not v1.** Adds: process supervision, lifecycle management, OS-specific autostart (launchd / systemd / Windows Service), error handling for "machine was asleep at 8 AM," scheduler config UI, and questions about how often / when. Manual button click satisfies P2 (minimum viable) and avoids all of this complexity.

**Build trigger.** Daily users explicitly ask for it. v1's manual flow has been validated for at least a month of real use.

---

## Notification channels (Slack / Discord / email / Telegram)

**What.** Push new strong-match postings to an external channel, so the user doesn't need to open the app to find out.

**Why we want it.** Some users will prefer push to pull. Reaches users at the moment a strong match drops.

**Why not v1.** Conflicts directly with the "calm morning ritual" thesis. Notifications create FOMO and urgency — the opposite of comfort. Push is also significantly more product-shaped than tool-shaped; we'd be designing for retention, not for the user's wellbeing. If anyone implements this, it should be off by default and clearly labeled as "not the recommended use."

**Build trigger.** Strong user demand AND a clear design that doesn't compromise the comfort thesis.

---

## Application tracker (Kanban / status board)

**What.** Track which postings the user applied to, current stage (지원완료 / 서류통과 / 1차면접 / 최종합격 / 불합격 / 자진사퇴), interview dates, follow-up reminders.

**Why we want it.** Closes the loop: tool helps find jobs AND helps manage applying to them. Tools like JobSync and Huntr exist for this — there's clear demand.

**Why not v1.** Different product surface (CRUD-heavy, statefulness-heavy, multi-page). v1 is "discovery + filtering"; application tracking is a separate problem with its own UX. Adding it to v1 would muddle the thesis.

**Build trigger.** v1 users report they're already using something else (Notion, spreadsheet) to track applications and want it integrated.

---

## Multi-language UI (English, etc.)

**What.** Externalize all UI strings to a translation file (`web/i18n/ko.json`, `web/i18n/en.json`). Allow language switching in settings.

**Why we want it.** Opens contribution surface to non-Korean speakers. Useful for foreign devs targeting Korean jobs.

**Why not v1.** Translation work is endless and the audience for v1 is Korean. *However:* v1 SHOULD externalize strings to a single file from day 1, even if only `ko.json` exists, so adding English later is a community PR rather than a refactor.

**Build trigger.** Externalization is a v1 architecture decision (not a feature). Actual English translation: any time a contributor offers it.

---

## Packaging: Homebrew tap, AUR, Nix, Docker image

**What.** First-class install on `brew`, AUR, Nix, and as a Docker container for users who prefer that.

**Why we want it.** Easier install = wider adoption. Each ecosystem has users who'll never download a tarball.

**Why not v1.** Each packaging target has its own maintenance overhead (formula updates, dependency tracking, signing). v1's install story is "download tarball, run binary" — sufficient for early adopters who are willing to follow a README.

**Build trigger.** v1 has 50+ stars AND someone in the community asks for a specific channel. Accept community-maintained formulas before maintaining any ourselves.

---

## ~~Broader experience-level inclusion when one source dominates~~ — SHIPPED 2026-05-27

**Resolution.** Shipped for 데모데이. The `any`-bucket post-filter combines (1) the experience-override parser (drop on 4+ year demand) with (2) an IT/dev keyword gate (drop on no dev signal in title+position). Sampled 200 rows: 3 dropped by (1), 117 dropped by (2), 80 kept (~40% survival), survivors are overwhelmingly dev roles.

Generic mechanism (per-source row cap, AI / embedding filter) deferred — the keyword-gate approach is good enough for v1 and the data didn't justify a heavier knob. Re-open this parking-lot entry if:

- A second source ships with the same dominant-bucket shape — that's when the generic solution earns its complexity.
- Users report the keyword gate is too noisy in practice (false positives like "사업개발" → engineer).
- The keyword set drifts (new dev specializations not in the regex / Korean list).

Original framing kept below for archaeology in case the v1.x decision needs to be revisited.

**Original framing.** Per-source policies (or a global mechanism) for safely including "no preference" buckets that would otherwise dominate the daily briefing. Specifically: 데모데이's `experience_level = "any"` carries ~720 of its 1000 rows — many genuinely 신입-friendly, but including them all would let one source flood the dashboard and break the calm-list thesis.

**Why we wanted it.** Excluding `any` outright (which is what the 2026-05-27 scraper did at first) lost real coverage. There are 신입-friendly postings hiding in that bucket that the user never sees. Same shape may show up on future sources (any aggregator that supports a "no preference" tag).

**The shipped answer** went with a stricter version of option 3: combine the override-parser drop with a small IT-keyword whitelist. Pure option 3 produced 98.5% survival (too lenient — flooded with marketing/design jobs). Keyword gate brought it to 40% (mostly dev roles).

---

## 네이버페이 (네이버파이낸셜) careers scraper

**What.** Scrape `recruit.naverfincorp.com` (the NAVER FINANCIAL Careers portal — covers 네이버페이 결제·대출·보험·카드·증권·부동산 hiring) for 신입 IT postings.

**Why we want it.** Originally identified as the highest-EV company-careers target after the 네이버 main-careers attempt failed yesterday — 네이버페이 is widely considered a top destination for new-grad fintech engineers in Korea.

**Why not v1 (or v1.6).** Recon on 2026-05-28 found two blockers:

1. **Zero active postings.** The `entTypeCdArr=0010` (신입) filter returns 0 rows. Even with no filter, the list page renders "진행 중인 공고가 없습니다" (no postings in progress). Implementing a scraper against an empty source is pure overhead.
2. **Same Saramin-style portal as 네이버 main-careers.** The listing is at `/rcrt/list.do`, click handlers are `onclick="show(annoId)"` (JS POST to `/rcrt/view.do`), and the deep-link pattern in `copyUrl()` is `/rcrt/view.do?annoId=X&lang=ko` via GET. Couldn't verify whether the GET form actually renders the posting body (no live data to test against). This is exactly the architecture we deferred 네이버 careers for yesterday.

**Re-recon trigger.** Re-check `recruit.naverfincorp.com/rcrt/list.do?entTypeCdArr=0010` in 3-6 months. If the 신입 count is non-zero, verify the deep-link via real browser click-through against `/rcrt/view.do?annoId=X&lang=ko`. If the body renders, the scraper is viable; if it redirects to `/rcrt/list.do` (the 네이버 main-careers failure mode), defer permanently. JSON API: `GET /rcrt/loadJobList.do?entTypeCdArr=0010&firstIndex=0` returns `{result, list[], totalSize}`; robots.txt 404s (unrestricted per RFC 9309).

---

## Toss (토스) careers scraper

**What.** Scrape `toss.im/career/jobs` for 신입 IT postings. Backing ATS appears to be Greenhouse (URL pattern is `?job_id=N`, similar to gh_jid).

**Why we want it.** Toss is a top-tier Korean fintech destination; 신입 engineers consider it a primary target.

**Why not v1 (or v1.6).** Recon on 2026-05-28 found two compounding reasons:

1. **Zero IT/dev 신입 postings.** Of 229 active postings on `/career/jobs`, only 6 carry a 신입 or 주니어 tag — and **all 6 are Customer Service, Sales/MD, Product Operations, Securities Settlement, or 온라인검수** roles. Not a single dev/engineering 신입 listing. Toss is hiring extensively, but new-grad pipeline appears to be elsewhere (referrals, internal program).
2. **robots.txt discourages job-detail crawling.** The bare listing `/career/jobs` is explicitly allowed, but `Disallow: /career/jobs?*` blocks any filtered listing, and `Disallow: /career/job-detail?gh_jid=5599901003` signals intent that individual job pages should not be crawled (even though our project's literal prefix matcher would let `/career/job-detail?job_id=X` through). A respectful crawler should honor the intent, not just the literal rule.

**Re-recon trigger.** Re-check the 신입/주니어 dev tag count in 6 months. If IT/dev new-grad listings appear AND Toss revises robots.txt to clarify the job-detail policy, revisit. Otherwise leave deferred.

---

## 쿠팡 (Coupang) careers scraper

**What.** Scrape `www.coupang.jobs/kr/jobs/` (the Korean-language Coupang careers portal) for 신입 IT postings.

**Why we want it.** 쿠팡 is one of Korea's largest tech employers; expected meaningful 신입 IT pipeline.

**Why not v1 (or v1.6).** Recon on 2026-05-28 found a hard structural blocker: **Cloudflare anti-bot challenge.** Both `https://www.coupang.jobs/` and `https://www.coupang.jobs/kr/jobs/` return HTTP 403 with the "Attention Required! | Cloudflare" challenge page — for both a vanilla `curl` and a gstack headless Chromium. robots.txt is permissive (`User-agent: * Allow: /`), so the blocker isn't policy; it's anti-bot infrastructure.

This is structurally identical to 원티드 (already noted under "Additional scrapers" above). Bypassing Cloudflare reliably needs either a paid residential-IP proxy, a stealth-headless browser with anti-detection (puppeteer-stealth, Playwright Firefox with fingerprint masking), or Cloudflare's official scraping allowlist — none of which fit v1's pure-Go, no-CGO, no-external-dependency design.

**Re-recon trigger.** When the project adopts an opt-in stealth-headless scraping path (e.g. a `JOBCRON_USE_HEADLESS=1` flag that activates a Chromium subprocess), 쿠팡 and 원티드 become viable together. Until then, hard-blocked.

---

## macOS code signing & notarization

**What.** Sign the macOS binary with an Apple Developer ID and notarize it so Gatekeeper doesn't warn on first run.

**Why we want it.** Gatekeeper warning ("cannot be opened because it is from an unidentified developer") is a real install-friction point and a trust signal.

**Why not v1.** Requires a paid Apple Developer account (~$99/year). v1's README will document the workaround (`xattr -d com.apple.quarantine ./jobcron` or right-click → Open).

**Build trigger.** Project has stable maintainership AND budget. Could also be funded by a sponsorship / GitHub Sponsors.

---

## Stage-2 AI delta recompute on an edited JD

**What.** When an employer edits a posting's description after it was first scraped, re-run the Stage-2 AI *delta* (the evidence-cited `AI 분석` signals), not just the Stage-1 extraction.

**Why we want it.** T7 (2026-06-08) added a bounded edited-JD re-fetch: a changed JD now flows new `content_hash` → fresh Stage-1 extraction → re-score. But the Stage-2 delta cache is keyed on `(posting_id, ai_input_hash, ai_version)` where `ai_input_hash` is the *goal text* only — the JD content is not in the key. So after a JD edit, the Stage-1 career/education facts refresh but the Stage-2 delta (which cites the JD) stays computed against the old text. Its quotes could even cite text no longer present.

**Why not now.** The clean fix is to fold the posting's `content_hash` into the Stage-2 cache key, which makes a JD edit a natural cache miss that the scrape-time auto-rate / 재평가 recompute. That's a migration + key change touching `ai_scores`, `UpsertAIScore`/`AIScore`/the batch reads, the T5 prune, and `scoreAll`'s merge — bigger than T7's scope. Lower priority because JD edits are infrequent and the stale delta degrades gracefully (the gate already verified its quotes against the JD-at-the-time; a now-missing quote just means a slightly stale chip until the next goal edit / model switch rotates the key).

**Build trigger.** If JD edits turn out to be common enough that stale Stage-2 chips are noticed, or when `ai_scores` is next migrated for another reason (bundle the key change in).
