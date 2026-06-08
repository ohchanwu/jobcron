# AI dial tuning — measured, not guessed (2026-06-08)

The v2.0 AI path has five tuning dials that were all originally *guessed*. This
file records the first real measurement of each against the live Anthropic
provider (model `claude-haiku-4-5-20251001`, the BYOK default) and the real
395-posting corpus, so they aren't re-derived from scratch next time.

**TL;DR.** One dial changed (`maxOutputTokens` 1024 → 2048). Four were kept with
evidence. The biggest discovery was *not* a dial: Stage-2 `ScoreDelta` JSON
parses fail **~15–45% of the time** live, from model JSON malformation — see
[The real problem](#the-real-problem-stage-2-scoredelta-json-malformation).

## Method

- A consistent snapshot of the real `jobs.db` (`sqlite3 .backup`) so the running
  app and its `ai_usage` ledger were never touched. The live calls spent tokens
  on the BYOK key (required — this session's live-AI bar) but did **not** write
  the real ledger.
- A throwaway harness (`cmd/aimeasure`, since deleted) that: (A) computed the
  assembled-model-text rune length over all 395 postings; (B) ran a *serial*
  per-call pass (clean per-call input/output tokens + latency, rate limiter not
  binding); (C) ran a *concurrent burst* mirroring a 재평가 — N postings ×
  (Extract + ScoreDelta) through a 6-worker pool sharing the 1 req/s limiter — to
  surface 429s and the pool's effective throughput.
- A throwaway diagnostic (`internal/ai/rawdump_integration_test.go`, since
  deleted) that dumped raw `ScoreDelta` replies to identify the malformations.

## Historical `ai_usage` ledger (real DB, read at run start)

| UTC day | input tokens | output tokens |
|---|---|---|
| 2026-06-03 | 607,567 | 79,853 |
| 2026-06-05 | 198,848 | 24,871 |
| 2026-06-06 | 174,635 | 34,319 |

06-03 is the Stage-1+Stage-2 build/test day (heavy, includes test traffic).
05/06 are real-use days. No 06-07/06-08 entries — AI hadn't run those days.

## Corpus size (Part A — no API)

Assembled model text (`ai.rawModelText`), rune length over all 395 postings:

```
min=60  p50=1562  p90=3230  p99=4562  max=6143
truncated at maxModelTextRunes(12000) = 0 / 395
> 8000 runes: 0    > 6000 runes: 1    > 4000 runes: 12 (3.0%)
```

Measured input-token ratio ≈ **0.28–0.36 tokens/rune** (Korean+English), plus a
fixed ~420-token system-prompt floor for Extract / ~480 for ScoreDelta.

## Per-call cost + latency (Part B, serial — pure, limiter not binding)

| call | input tokens | output tokens | latency |
|---|---|---|---|
| Extract | 483–3335 (p50 ~1785) | **61–117** (p50 ~80) | p50 1.4s, max 2.8s |
| ScoreDelta | 960–3812 (p50 ~2262) | **415–1024⚠** (successful p50 ~685, max ~820) | p50 4.9s, max 5.8s |

⚠ One real ScoreDelta reply hit **exactly 1024 output tokens** and failed with
`unexpected end of JSON input` — the old `maxOutputTokens=1024` truncated the
reply mid-JSON and the whole delta was dropped.

## Concurrency (Part B2 — 6-worker burst, shared 1 req/s limiter)

| spacing | calls | wall-clock | effective | 429s |
|---|---|---|---|---|
| 1.0s | 40 | ~43s | ~0.95 calls/s | 1–2 |
| 1.5s | 48 | ~76s | ~0.63 calls/s | 0 |

The effective rate at 1.0s (~0.95 calls/s) sits right at the 1 req/s limiter, so
**the limiter — not the pool — is the bottleneck with 6 workers.**

---

## The five dials — verdicts

### 1. `maxOutputTokens` (`anthropic.go`) — **CHANGED 1024 → 2048**

Stage-1 Extract replies are tiny (max ~120 output tokens). Stage-2 ScoreDelta
carries an evidence quote per signal and runs large: successful replies reached
~820 tokens and at least one **hit the 1024 ceiling and truncated mid-JSON,
dropping the whole delta**. Raised to 2048 (≈2.5× the largest successful reply).
After the change a corpus burst produced ScoreDelta outputs up to ~1090 tokens
that now complete. Billing is per *actual* output token, so the headroom is free
for the common case; the cap only ever bites a reply that would exceed it.

### 2. `aiRequestSpacing` (`provider.go`) — **KEEP 1.0s** (comment corrected)

1 req/s = ~60 req/min when the pool keeps it saturated. The original comment
claimed this was "well under Anthropic's tier-1 ~50 req/min ceiling" — that math
is wrong (60 > 50); corrected in code. Measured: ~1–2 HTTP 429s per 40-call
burst at 1.0s, **0 at 1.5s**, but 1.5s costs ~50% wall-clock. The 429s are
occasional (not persistent) and almost certainly **input-tokens-per-minute**
driven (~2k input tokens/call → ~120k ITPM at 60 req/min). Kept at 1.0s because:
real re-rates are small (a 429 is rare in daily use, only sustained bursts trip
it), a 429 is not fatal (the row retries on the next press — see
`RERATE_NOTES.md` case B), and scrape-time auto-rate now makes re-rate latency a
per-scrape cost worth keeping low. **If a future tuner prefers completeness over
speed, 1.5s spacing reliably clears the 429s.**

### 3. `rerateWorkers` (`server/rerate.go`) — **KEEP 6**

The burst's effective rate (~0.95 calls/s) ≈ the 1 req/s limiter, i.e. 6 workers
already saturate the limiter — more workers can't go faster (limiter-bound).
With per-call API latency ~1.4s (Extract) + ~4.9s (ScoreDelta), keeping the 1/s
pipe full needs ~5 in-flight calls; 6 gives a small margin. Below ~5 workers the
pool would start to under-saturate the limiter. 6 is right.

### 4. HTTP timeout (`client.go`, `newPinnedHTTPClient(host, 60s)`) — **KEEP 60s**

Max observed *pure* API latency was ~5.8s (ScoreDelta) — ~10× under the 60s
timeout. The timeout is a hung-connection catcher, not a throttle; 60s leaves
generous room for a slow call (or a larger model than Haiku) without spuriously
killing a legitimate request.

### 5. `maxModelTextRunes` (`extract.go`) — **KEEP 12000**

The whole corpus fits: max assembled text is 6143 runes (51% of the cap); **0 of
395 postings truncate.** This is a defensive cost ceiling, and a ceiling you
never hit is doing its job — tightening it toward the observed max would only
raise the chance of clipping a future outlier JD's eligibility section (which can
sit anywhere in a long body). `content_hash` is over the pre-truncation text, so
this can be retuned later without invalidating any cached extraction (S6) — but
there's no measured pressure to.

---

## The real problem: Stage-2 ScoreDelta JSON malformation

The dial tuning is minor next to this. **Live `ScoreDelta` replies fail to parse
~15–45% of the time** (input-dependent), and the failures are *stochastic* — the
same input parses fine on a retry. Confirmed root causes (raw replies captured):

1. **Prompt-induced `+N` integers.** The Stage-2 prompt says
   `"delta": <정수. 목표에 맞으면 +, 어긋나면 ->` ("+ if it fits"), which leads the
   model to emit `"delta": +3`. A leading `+` is **invalid JSON** →
   `invalid character '+' looking for beginning of value`. This was the dominant
   failure on the real corpus burst (9 of 11 burst errors).
2. **Malformed `forms` arrays / strings.** e.g.
   `["프론트엔드", "frontend", 프론트", "UI"]"` — a missing opening quote on a
   Korean element, a stray trailing `"`, a missing comma. Haiku is simply
   unreliable at strict JSON for the multi-item schema, especially the absence
   `forms` lists.

Extract (Stage 1, simple fixed schema) had **0 failures** in the same runs — only
the richer ScoreDelta schema malforms.

`RERATE_NOTES.md` already notes these land in "case B" (no `ai_scores` row, retried
next press) and recover — but a 15–45% first-pass drop is a real quality dent: a
fresh briefing's scrape-time auto-rate silently leaves many rows without their AI
chip until a later press happens to succeed. **Fix routed to T6** (parsing
hardening): tolerate `+N`, drop the prompt's `+` induction, and consider a single
retry on parse failure (cheap given the failures are stochastic).

---

*Measured 2026-06-08 against `claude-haiku-4-5-20251001` and a 395-posting
snapshot. The harness + raw-dump diagnostic were throwaway and removed; this file
is the durable record. Numbers will drift with model, corpus, and account tier.*
