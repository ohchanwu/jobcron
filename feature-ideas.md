# Feature Ideas — Parked

Things we discussed but explicitly cut from v1 to protect scope. Each entry should answer: what is it, why we want it, why it's not v1, and what triggers building it.

---

## Resume upload → auto-extract profile

**What.** Drop a resume (PDF / Markdown / plain text) into the profile-setup UI. App parses it, extracts stack tags, years of experience, education, location, etc. Shows the extracted profile in an editable preview. User confirms or edits, then saves.

**Why we want it.** Magical first-run experience. Lowers the cognitive cost of profile setup from "fill out a 20-field form" to "drop your resume here." Big "whoa" moment for new users and demo viewers.

**Why not v1.** Adds a whole new problem domain (PDF parsing, layout extraction, multilingual NER for Korean + English résumés). Parsing failures during first-run UX are catastrophic — the first impression breaks. Better to ship the structured form, see how people actually fill it, then build the parser against real shapes.

**Build trigger.** v1 has 50+ users AND structured-form completion rate is below ~70% (i.e. the form is friction). Then resume upload becomes the obvious unlock.

---

## AI / ML scoring layer

**What.** Replace (or augment) the current keyword-and-weight scorer with a semantic model — embeddings + cosine similarity between profile and posting, or an LLM that reads each posting and outputs a fit score with a short rationale. Could be local (gguf / ONNX) or hosted (Anthropic / OpenAI / Together).

**Why we want it.** Current matching is token-exact: "개발" doesn't match "개발자", "야근 없음" doesn't get distinguished from "야근", particle-attached forms like "야근이" miss "야근". A semantic model would close most of those gaps and pick up nuance the keyword matcher cannot (e.g. "성장하고 싶은 신입 환영" ≈ "신입 친화적").

**Why not v1.** Three reasons. (1) Breaks the "single-binary, no network at runtime, no API keys" distribution thesis — a hosted LLM means cost handling, key storage, rate limits, offline degradation. (2) Current scoring is intentionally legible — "why did this score 60?" has an obvious answer in the breakdown chips. Semantic scores are opaque ("the model said so"), which fights the calm-and-trustworthy product thesis. (3) You can't tell if an ML scorer is *actually* better without a labelled ground-truth set, and that set doesn't exist until v1 has been running long enough across multiple sources to accumulate "yes/no, was this a good match" signal.

**Build trigger.** v1 has been used for at least 2-3 months across multiple portals AND the user has explicit examples of "the keyword matcher should have flagged this but didn't" / "it scored this high but it's actually irrelevant." Then build a labelled eval set from real usage, then layer the model on top — never as a replacement, always as an additional signal alongside the keyword breakdown.

---

## Hybrid: structured form + free-form keyword overrides

**What.** Keep the structured profile form as the primary input, but add two free-form fields underneath: "반드시 포함해야 할 키워드" (must-include) and "절대 포함되면 안 되는 키워드" (dealbreakers). Real Korean job preferences have long-tail signals that don't fit any structured field: 병특 transferable, 자유 출퇴근, 노가다 거부, 적대적 완전 재택, 외국계, 스톡옵션, 외국어 사용 환경, etc.

**Why we want it.** The structured form alone can't capture every signal. Free-form keywords fill the long tail without bloating the form schema. Strict include/exclude semantics keep the scoring engine simple and predictable.

**Why not v1.** v1 is testing whether structured scoring against a fixed schema is *enough*. Adding free-form overlays before we know the baseline answer would muddy the signal. Also: dealbreaker keywords on long-form posting text need careful matching (token boundaries, negation handling) — that's its own small project.

**Build trigger.** v1 users report that the structured profile misses important signals. First implementation should be additive only — keyword overrides modify the score, they don't replace any structured field.

---

## Additional scrapers (원티드, Programmers, Saramin, JobKorea, direct company pages)

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

## Broader experience-level inclusion when one source dominates

**What.** Per-source policies (or a global mechanism) for safely including "no preference" buckets that would otherwise dominate the daily briefing. Specifically: 데모데이's `experience_level = "any"` carries ~720 of its 1000 rows — many genuinely 신입-friendly, but including them all would let one source flood the dashboard and break the calm-list thesis.

**Why we want it.** Excluding `any` outright (which is what the 2026-05-27 scraper does) loses real coverage. There are 신입-friendly postings hiding in that bucket that the user never sees. Same shape may show up on future sources (any aggregator that supports a "no preference" tag).

**Why not v1.** No good knob exists yet. The right design isn't obvious — candidates include:

- **Per-source row cap.** Each source contributes at most N postings to the briefing, weighted by relevance score. Naturally throttles dominant sources but needs scoring tuning to pick the right N postings.
- **User-configurable toggle.** A profile setting "include broad/no-preference 데모데이 listings" with off-by-default. Honest but adds another decision the user must understand.
- **Quality filter applied to broad buckets.** Use the experience-override parser already in `internal/scraper/experience.go` to drop `any` postings whose title says "5년 이상" or "시니어"; include the rest. Cheap, leans on existing infrastructure, doesn't add new UI.
- **Light AI / embedding filter** restricted to "broad buckets" → keep only ones whose title and JD actually read as 신입-friendly. Most expressive option but introduces an ML dependency.

The override-parser approach (option 3) is probably the right v1.x answer because it reuses what we already have. But the design call needs `/office-hours` before code.

**Build trigger.** Either the user reports the briefing feels too sparse on 데모데이 day-to-day, OR a second source ships that has the same dominant-bucket shape and the same problem becomes worth a generic solution.

---

## macOS code signing & notarization

**What.** Sign the macOS binary with an Apple Developer ID and notarize it so Gatekeeper doesn't warn on first run.

**Why we want it.** Gatekeeper warning ("cannot be opened because it is from an unidentified developer") is a real install-friction point and a trust signal.

**Why not v1.** Requires a paid Apple Developer account (~$99/year). v1's README will document the workaround (`xattr -d com.apple.quarantine ./job-scraper` or right-click → Open).

**Build trigger.** Project has stable maintainership AND budget. Could also be funded by a sponsorship / GitHub Sponsors.
