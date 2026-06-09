# Local model vs BYOK-Anthropic — research findings

_Research session 2026-06-09 (no daily-session; ad-hoc office-hours research). Multi-agent
fan-out: 7 dimension researchers + 4 adversarial verifiers + 1 synthesizer, ~708K tokens, 213
web tool calls. Sources listed at the end._

**Question.** The app scores Korean 신입 IT postings with BYOK Anthropic Haiku 4.5 over a
data-in/data-out `ai.Provider` seam. The user wants a **local** model to escape per-token cost,
Anthropic rate limits (429s), and the self-imposed 50-listing cap (`aiPerCallCap`) so they can
batch-re-rate 300+ saved postings freely — ideally with "a more capable model than Haiku."
Hardware: Apple M3 Pro, 18GB unified memory. Ollama 0.9.5 and LM Studio already installed.

**Method note.** Four load-bearing claims were handed to adversarial verifiers told to refute
them. Results: the "local fixes our JSON failures" claim was **refuted**, "good enough vs Haiku"
and "batch finishes in well under a few hours" were **partly supported** (corrected below), and
"embed-weights-in-binary is a dead end" was **supported**. The body reflects the corrected
claims, not the optimistic originals.

---

## Bottom line

If you want a local model, the move is a **sidecar HTTP server (Ollama on `127.0.0.1:11434`), not embedded weights** — and you add it as one more `ai.Provider` behind the seam you already have, leaving the 22MB CGO-free single binary untouched. The strongest commercial-clean pick that fits 18GB and reads Korean well is **Qwen2.5-14B-Instruct (Apache 2.0, ~9GB Q4)** with **Kanana 1.5 8B (Kakao, Apache 2.0, ~5GB Q4)** as the lighter, faster runner-up. The honest verdict is split: a local 14B is **comparable to Haiku for Stage-1 fact extraction but clearly worse for the Stage-2 cited score-deltas**, where open-weight models invent unsupported claims at materially higher rates and your `GateDelta` gate converts that into a thinner, more conservative AI layer. The single most important caveat: **the thing a local model was supposed to fix — the ~15-45% Stage-2 JSON-parse failures — is already fixed on Haiku** (the T6 parser+prompt change on 2026-06-08, ~6/12 to 12/12), so do not adopt local inference to solve a JSON problem you no longer have. Treat this as a _cost/offline/privacy_ decision (no per-token billing, runs without network), not a quality upgrade.

## The model question — what actually fits 18GB and reads Korean well

The budget is ~10-12GB for weights + KV cache on an 18GB unified-memory M3 Pro, at Q4/Q5, sized 7B-14B. Four candidates clear the commercial-license bar (the two best _raw_ Korean models, LG's EXAONE-3.5/4.0, are disqualified up front — see Licensing).

| Model                         | Params | Q4_K_M size                                               | Korean standing                                                                                                                      | License                                                           | JSON / structured-output                                                                                                                       |
| ----------------------------- | ------ | --------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| **Qwen2.5-14B-Instruct**      | 14.7B  | ~9.0GB ([Ollama](https://ollama.com/library/qwen2.5:14b)) | mid-pack vs Korea-native, but adequate; 29+ languages                                                                                | **Apache 2.0** (14B size specifically)                            | Best-documented; explicitly trained for "structured data understanding" and JSON ([HF card](https://huggingface.co/Qwen/Qwen2.5-14B-Instruct)) |
| **Kanana 1.5 8B** (Kakao)     | 8B     | ~5GB                                                      | Class-leading: HAERAE ~85 (vs Qwen2.5-7B 67.5), KMMLU ~50.6, IFEval ~7.8 ([HF](https://huggingface.co/kakaocorp/kanana-1.5-8b-base)) | **Apache 2.0**                                                    | Strong instruction-following; less battle-tested for tool/JSON than Qwen                                                                       |
| **Mi:dm 2.0 Base 11.5B** (KT) | 11.5B  | ~7.0GB ([arXiv](https://arxiv.org/html/2601.09066v1))     | Korea-centric, KMMLU 47.67, HAERAE 78.19, 32K ctx                                                                                    | **MIT**                                                           | Untested; GGUF builds are community-repackaged (verify tokenizer/chat template)                                                                |
| **Mistral-Nemo 12B**          | 12B    | ~7GB                                                      | generic multilingual, no Korean specialization                                                                                       | **Apache 2.0** ([Mistral](https://mistral.ai/news/mistral-nemo/)) | solid, but Korean is the weak axis                                                                                                             |

**Best default pick: Qwen2.5-14B-Instruct.** It is the only candidate that is simultaneously Apache-2.0-clean, has the strongest documented JSON/structured-output track record (the capability your two-stage pipeline leans on hardest), carries a 128K context window (your assembled model text maxes at 6,143 runes, so context is never the bottleneck), and is among the most quantization-tolerant families — Q4_K_M retains ~95-98% of full-precision accuracy with minimal instruction-following loss. Its one real weakness is that it is only mid-pack on _Korean_ relative to the Korea-native models, but the verification confirms Korean is not the bottleneck for this task.

**Runner-up: Kanana 1.5 8B.** Half the memory (~5GB vs ~9GB), the best Korean-quality-per-GB in the field, mature enough to run comfortably, and the path that is genuinely comfortable on 18GB rather than tight. If you find the 14B fit too cramped against a running browser, or you want the corpus batch to finish in ~3h instead of ~4.5h, drop to Kanana (or another Apache 8B) and accept a small Korean-instruction-following trade.

Mi:dm 2.0 has the most appealing license (MIT) and is genuinely Korea-centric, but it is recent (technical report Jan 2026) and its GGUF builds are community-repackaged rather than first-party — verify the GGUF tokenizer and chat template are correct before relying on it. Treat it as a "prototype and measure" option, not a default.

## Can a local model match Haiku for this task

No single verdict covers the whole pipeline — the honest answer splits along your two stages, and stapling them together (as the original research claim did) overstates the local model.

**Stage-1 fact extraction (career/education JSON facts): comparable.** This leans on instruction-following + JSON formatting + short reasoning, where the gap is modest. Anthropic's own card puts Claude 3.5 Haiku IFEval at **85.9** vs **81.0** for Qwen2.5-14B ([Anthropic model card](https://www-cdn.anthropic.com/c7822cdc35ad788ec87e14b3a9d45010f1f86c38.pdf)) — about 5 points, "modestly ahead, not a chasm." On general reasoning a head-to-head calls the two "evenly matched": Haiku wins HumanEval (88.1/83.5) and MMLU-Pro (65.0/63.7), Qwen wins MATH (80.0/69.2) and GPQA-Diamond (45.5/41.6) ([llm-stats](https://llm-stats.com/models/compare/claude-3-5-haiku-20241022-vs-qwen-2.5-14b-instruct)). Q4 quantization does not break this half. Korean is not the bottleneck — a Korean-native 8B (EXAONE-3.5-7.8B) actually beats Qwen2.5-7B on Korean instruction-following (KoMT-Bench 7.96 vs 5.19; LogicKor 9.08 vs 6.38, [arXiv](https://arxiv.org/html/2412.04862v2)).

**Stage-2 cited score-deltas (grounding/faithfulness): clearly worse, but usable behind the gate.** This is where a local model genuinely loses. Faithfulness research finds open-weight baselines "exhibit the weakest faithfulness, often adding plausible but unsupported requirements," with open-model hallucination rates roughly 15-30% versus ~3% for Claude-class models ([arXiv](https://arxiv.org/pdf/2501.00269)). That is precisely the capability your `GateDelta` (`internal/ai/score_delta.go`) exists to catch — it requires the cited evidence token to actually appear in the JD. With a local model the gate fires _more often_, so the failure mode is **more rejected/empty AI chips, not confidently-wrong scores** — a safe failure mode, but a real quality drop. The net is a thinner, more conservative AI layer than Haiku produces.

Two corrections worth holding onto. First, the baseline is moving: your provider runs **Haiku 4.5** (`claude-haiku-4-5-20251001`), which is stronger than the Haiku 3.5 most of these benchmarks compare against, so the gap a local 14B must close is wider than the 3.5 numbers suggest. Second, EXAONE-3.5 — the best _raw_ Korean model and a tempting co-pick — is **disqualified** for a distributed app by its non-commercial license, so Qwen2.5-14B is the only viable 14B exemplar here.

## The JSON-reliability angle

The original hypothesis was: _a local server can hard-guarantee schema-valid JSON via grammar-constrained decoding, which the Anthropic API cannot, so the ~40% Haiku parse-failure problem disappears._ **The first half is true; the contrast and the motivation are not.**

Grammar-constrained decoding is real and strong. Ollama's native `format` field (since v0.5, Dec 2024) accepts a full JSON Schema object, compiles it to a GBNF grammar, and masks any token that would break the grammar during sampling — the emitted bytes are **guaranteed structurally valid JSON matching the schema's shape** ([Ollama docs](https://docs.ollama.com/capabilities/structured-outputs), [mechanism writeup](https://blog.danielclayton.co.uk/posts/ollama-structured-outputs/)). llama.cpp's `llama-server` offers the same via true `json_schema` `response_format` and GBNF. Use the **native `/api/chat` `format` field**, not the OpenAI-compat `/v1/chat/completions` `response_format` — Ollama's own docs document the native schema path far more fully, and there is a known gap where Ollama wants the schema as a top-level `format` rather than OpenAI's nested `{type:'json_schema', json_schema:{...}}` ([issue #10001](https://github.com/ollama/ollama/issues/10001)). Set `temperature: 0` and instruct the model to emit JSON in the prompt to avoid runaway whitespace.

But three facts dismantle the motivation:

1. **Anthropic can hard-force schema-valid JSON too, as of mid-2026.** Structured Outputs is GA, compiles JSON schemas into a constraining grammar, and is available for Claude Haiku 4.5 — the exact model this app uses. Hard schema enforcement is no longer unique to local servers.
2. **The problem was already fixed on the hosted path.** Your own `internal/ai/AI_TUNING_NOTES.md` measured the failure at **~15-45%, Stage-2 only (Stage-1 had 0 failures)** — so the "~40%" is the high end of a real band, and it was confined to the richer `ScoreDelta` schema. The dominant cause was _self-induced_: the Stage-2 prompt told the model to emit `"delta": +3` (a leading `+` is invalid JSON), plus malformed Korean `forms` arrays. **T6 (2026-06-08) resolved it** with a parser fix (strip invalid leading `+`, depth-aware span, accept bare top-level arrays) and a prompt change — a live re-check parsed **12/12 vs ~6/12 before**, on Haiku, with no local model.
3. **Structure is not accuracy.** Even where grammar constraints apply, they guarantee the _shape_, not that values are correct or non-hallucinated — one cited study showed constrained decoding at 91.37% accuracy vs 93.63% free-form, i.e. constraining can _lower_ semantic accuracy. You would still need `GateDelta` to validate the cited evidence app-side, exactly as today.

So: grammar-constrained decoding is a genuine nice-to-have for a local provider, but it is **neither necessary nor uniquely capable**, and it does not justify the project on JSON grounds.

## Speed and the batch-the-corpus dream

The "run it unattended and it grinds the whole corpus" UX is **real, but it's an overnight (multi-hour) job, not "well under a few hours" — and the 14B case is the slow one.**

Measured and derived throughput on M3 Pro (~150 GB/s memory bandwidth, ~half the 300 GB/s of a 30-core M3 Max; generation is bandwidth-bound):

- **7B-8B Q4**: ~25-30 generation t/s, ~250-270 t/s prompt-processing. The 7B Q4_0 figure (30.65 t/s) is **directly measured** on M3 Pro ([llama.cpp Discussion #4167](https://github.com/ggml-org/llama.cpp/discussions/4167)).
- **14B Q4**: ~18-20 generation t/s, ~120-140 t/s prompt-processing. This is **bandwidth-scaled from M3 Max, not directly measured** (no primary M3 Pro 14B benchmark exists); an Apple-Silicon benchmark site for this exact config puts 14B-class at ~10-20 t/s, consistent with or below the scaled estimate.

Wall-clock for **300 postings × (~1,500 input + ~800 output tokens)**, generation-dominated:

- **8B model: ~3.0 hours** — `(1500/260 + 800/27) × 300 ≈ 35s/call`.
- **14B model: ~4.5 hours central** — `(1500/130 + 800/19) × 300 ≈ 54s/call`. Realistic band **~3.5-6h**: optimistic floor ~3.55h (MLX +25%, cool), realistic ~5.5h once the KV cache grows over the 800-token output (8B drops 50.7→36.1 t/s from 1K to 8K context), pessimistic 6-8h with thermal throttling on a fan-limited MacBook.

Three honest corrections to the dream: (1) "well under a few hours" is wrong for 14B — the central estimate (~4.5h) _is_ "a few hours," and even the best case (~3.55h) isn't "well under" it; (2) the fast ~3h figure belongs to the **8B** path, the only one that approaches the claim; (3) the 14B-on-18GB fit is tight (~8.5GB weights + ~1-1.5GB KV cache for 8K context + macOS/app overhead ≈ 10-11GB of 18GB), and the digest warns it "may swap or be killed" if a browser and the app are running — which undercuts "unattended" reliability for 14B specifically.

Net: viable overnight batch, comfortably so at **8B (~3h)**; at **14B (~4.5h central, 3.5-6h+ band, tight memory)** budget it as a true overnight run with little else open. There is no per-call network or rate-limit cost locally, which is the genuine win over the rate-limited Haiku path (your re-rate already throttles to ~1 req/s and sees occasional 429s on bursts).

## Architecture — why "bundled" is the wrong word

**Embedding the weights in the binary is a dead end, on two independent grounds**, and the verification confirms both:

1. **No pure-Go engine runs a useful modern LLM at usable speed.** Every genuinely pure-Go inference library is either the wrong model class or a toy: spago/cybertron are BERT/BART/PEGASUS-era encoder/seq2seq (last release Nov 2023), not instruction-tuned decoders; `tmc/go-llama2` is usable only to ~42-110M params; `gotzmann/llama.go` is a 2023 FP32 LLaMA-1 port (32GB+ RAM, abandoned). GoMLX is the strongest candidate but its Gemma path imports the **XLA/PJRT native C++ backend**, not the pure-Go `SimpleGo` backend, which the project itself calls "very portable but slow" with zero LLM throughput numbers. Tellingly, **Ollama itself removed its in-process CGO engine** ([PR #16031](https://github.com/ollama/ollama/pull/16031), merged 2026-05-29) and now runs `llama-server` as a **subprocess** — the industry-leading Go LLM project chose sidecar over in-process CGO.
2. **The "22MB binary" framing fails on weights size alone.** A useful 7B-14B model at Q4 is **4.5-9.3GB of weights** — that cannot fit in a 22MB binary regardless of engine.

The middle option (purego + FFI to a bundled native llama.cpp lib — yzma, gollama.cpp) is `CGO_ENABLED=0` at _build_ time, but the matmuls still run in a foreign-compiled C++ artifact, each platform needs its own native lib (so it cannot cross-compile from one source the way your CI's static linux/arm64 + darwin/arm64 build does), and it isn't "pure Go." It collapses into the llama.cpp path. The project's actual reason for the CGO-free constraint — single-source cross-compile — rules it out.

**The right move: a new local-HTTP provider behind the existing `ai.Provider` seam, pointed at a localhost server.** Your `providerSpec` chassis already supports this — it's the same seam OpenAI slotted into before it was removed. A `local` (or `ollama`) provider implements `Name() + Extract() + ScoreDelta()`, talks to `http://127.0.0.1:11434/api/chat`, and `ReconfigureAI` wires it from the profile's `AIProvider`/`AIModel` exactly as it does Anthropic today. **The 22MB CGO-free single binary is preserved** — the model runs in a process the user already has, not in your binary.

One integration note specific to your code: the current Anthropic transport pins `DialContext` to one remote host for egress safety. A localhost sidecar (`127.0.0.1:11434`) is a different security posture and needs its own localhost-specific dial path, not a reuse of the single-remote-host pin.

**Server choice: target Ollama.** Among the four mature local OpenAI-compatible servers (Ollama :11434, LM Studio :1234, llama.cpp `llama-server` :8080, Jan :1337), Ollama is the only one that can be **safely assumed always-on**: it auto-starts a background daemon on login on macOS, binds `127.0.0.1:11434`, has a trivial HTTP health check (`GET /api/version` or `GET /` → "Ollama is running"), loads models just-in-time on first request, and offers a REST model-manager (`pull`/`list`/`load`) so you never shell out to a CLI ([Ollama docs](https://docs.ollama.com/faq)). LM Studio's server is GUI-gated by default; `llama-server` is the most spec-complete but is a manually-launched per-model process with no daemon — wrong shape for a zero-config sidecar.

**Require / detect / auto-start: detect-and-instruct with thin best-effort recovery.** Don't hard-require the user to start it (poor UX when it's usually already up), and don't silently force auto-start (fragile across installs, can fight the user's own Ollama usage). Health-check `GET /api/version`; if down, attempt one best-effort `ollama serve` spawn; if that fails, surface a clear instruction with the exact command and the model to pull. This keeps AI off-by-default (your `Server.ai == nil` invariant) and degrades gracefully when no local server is present — scoring stays byte-identical to the no-AI path.

## Licensing and first-run UX

**Safe to recommend / auto-pull (permissive, no scale cap, no field-of-use restriction):** Qwen2.5 **7B and 14B** (Apache 2.0 — note 3B and 72B use the custom Qwen-Research/Qwen license, so pin 7B/14B), Qwen3 across sizes (Apache 2.0), Kanana 1.5 8B (Apache 2.0), Mi:dm 2.0 (MIT), Mistral-Nemo 12B (Apache 2.0), Trillion-7B (Apache 2.0 — but its 4K context is too small for full JDs, so not a pick here).

**Avoid for a distributed app:** **EXAONE — all versions** (EXAONE AI Model License "-NC", commercial/revenue use explicitly prohibited, [LICENSE](https://github.com/LG-AI-EXAONE/EXAONE-3.0/blob/main/LICENSE)) and **SOLAR-10.7B-Instruct** (CC-BY-NC-4.0). This is what disqualifies EXAONE despite it topping the Korean instruction-following benchmarks.

**Ship-with-care:** **Gemma** (custom Gemma Terms — commercial use allowed but you must pass the Terms + Prohibited Use Policy through to every recipient, and Google reserves a remote-restriction right; not OSI-open) and **Llama 3.x** (700M-MAU cap, "Built with Llama" attribution, a name-prefix rule). For a small local-first app both are _usable_ but carry pass-through obligations even on auto-download — surface the notice in-app if you go this route. Verify the actual LICENSE file in the specific repo/size you pull; HF "license" metadata is the publisher's tag, not the contract, and it varies per size and per variant.

**First-run download reality:** a 7B Q4_K_M gguf is **~4.5-4.7GB**, a 14B Q4_K_M is **~9.3GB** ([Ollama qwen3:14b](https://ollama.com/library/qwen3:14b-q4_K_M)). Download time: ~6 min at 100 Mbps, ~11 min at 50 Mbps, ~55 min at 10 Mbps for a 7B (roughly double for 14B). `ollama pull` on first run is workable but a footgun if done as a silent blocking spinner — a multi-GB wait reads as "broken," and `ollama pull` has a known stall mode on its Cloudflare-R2 backend ([issue #11312](https://github.com/ollama/ollama/issues/11312)). Make it **explicit, consent-gated, progress-bar'd, and resumable** (read the `POST /api/pull` stream to completion and key on the terminal `{"status":"success"}`, not the first 200). This fits the project's calm-UX thesis — a blocking multi-GB download with no progress is the opposite of "makes a stressed 신입 feel calmer."

## Open questions and risks

- **The load-bearing metric is unmeasured: JSON-validity rate and field-level extraction accuracy on a held-out set of _your own_ Korean postings.** No published Korean benchmark (KMMLU, HAERAE, LogicKor, KoMT-Bench) measures schema adherence on your specific extraction prompt. **Spike needed:** run `internal/scoring/testdata/qa_postings.json` through Qwen2.5-14B (Ollama, native `format` schema, temp 0) and your `GateDelta` gate, and compare Stage-1 extraction accuracy and Stage-2 surviving-delta count against Haiku 4.5. This is the only thing that settles "good enough."
- **Stage-2 quality drop is directionally certain but magnitude-uncertain.** The ~15-30% open vs ~3% Claude hallucination figures are blended cross-task practitioner numbers, not measured on Korean JD extraction with citations. The _direction_ (open grounds worse) is reliable; the size for your task is not.
- **The 14B M3 Pro generation figure (~18-20 t/s) is derived, not measured.** No primary M3 Pro 14B benchmark exists; real numbers could land anywhere in ~10-22 t/s. The 8B M3 Pro figure is directly measured and trustworthy.
- **14B on 18GB is a tight fit** (~10-11GB of the pool at 8K context) and "may swap or be killed" with a browser open. If you adopt local, **Kanana 8B is the safer default on this exact machine**; reserve 14B for when little else runs.
- **Mi:dm 2.0 GGUF builds are community-repackaged** — verify the tokenizer and chat template before trusting it, or stick to Qwen2.5/Kanana which have mature, widely-used distributions.
- **Ollama's JSON-Schema → GBNF support is a subset.** Deeply nested schemas, `oneOf`/`anyOf`/`allOf`, regex patterns, and unconstrained free-text string fields can misbehave (there are open repetition-loop issues on free-text fields). Keep the schema tight and prefer enums/bounded types — which your Stage-1 schema mostly already is.
- **The motivating problem is gone.** The JSON-parse failure that made local inference look attractive was fixed on Haiku (T6, 2026-06-08). Re-confirm _why_ you want local — cost, offline operation, privacy, or no-rate-limit batch — because it is **not** a quality or reliability upgrade over the current hosted path, and on Stage-2 it is a downgrade.
- **This is a `feature-ideas.md`-class scope question, not a bug fix.** Local-LLM sidecar support is a new capability; confirm it's actually in scope (and check whether `feature-ideas.md` already has a position) before building past the spike.

---

## Adversarial verification verdicts

The four load-bearing claims, after fresh-eyes refutation attempts:

1. **REFUTED** — "Local grammar-constrained decoding hard-guarantees JSON the Anthropic API can't, so the ~40% parse failures disappear." Anthropic now has GA Structured Outputs for Haiku 4.5; the failures were already fixed via T6 parser+prompt; grammar constrains shape not accuracy.
2. **PARTLY SUPPORTED** — "A local 14B is good-enough vs Haiku." True for Stage-1 extraction (comparable), false for Stage-2 cited deltas (open models ground worse; GateDelta makes it fail safe but thinner).
3. **PARTLY SUPPORTED** — "Batch 300 postings finishes well under a few hours." True for 8B (~3h); 14B is ~4.5h central (3.5-6h band) and memory-tight — an overnight job, not "well under a few hours."
4. **SUPPORTED** — "Embed-weights-in-binary is a dead end." No production pure-Go path; weights alone are GBs; even Ollama moved to a subprocess engine. Local = sidecar, full stop.

## Sources

Ollama / serving: ollama.com/library/qwen2.5:14b · docs.ollama.com/capabilities/structured-outputs · docs.ollama.com/api/openai-compatibility · docs.ollama.com/faq · ollama.com/blog/structured-outputs · github.com/ollama/ollama/issues/10001 · github.com/ollama/ollama/issues/11312 · github.com/ollama/ollama/pull/16031 · blog.danielclayton.co.uk/posts/ollama-structured-outputs/ · lmstudio.ai/docs/developer/core/server · github.com/ggml-org/llama.cpp/blob/master/tools/server/README.md · jan.ai/changelog/2026-05-22-jan-v0.8.0

Models / Korean: huggingface.co/Qwen/Qwen2.5-14B-Instruct · huggingface.co/kakaocorp/kanana-1.5-8b-base · arxiv.org/html/2601.09066v1 (Mi:dm 2.0) · huggingface.co/LGAI-EXAONE/EXAONE-3.5-7.8B-Instruct · arxiv.org/html/2412.04862v2 (EXAONE-3.5) · mistral.ai/news/mistral-nemo/ · huggingface.co/trillionlabs/Trillion-7B-preview · benchlm.ai/leaderboards/korean-llm

Quality / faithfulness: www-cdn.anthropic.com/c7822cdc35ad788ec87e14b3a9d45010f1f86c38.pdf (Haiku card) · llm-stats.com/models/compare/claude-3-5-haiku-20241022-vs-qwen-2.5-14b-instruct · arxiv.org/pdf/2501.00269 (faithfulness) · github.com/vectara/hallucination-leaderboard · artificialanalysis.ai/models/claude-4-5-haiku

Speed: github.com/ggml-org/llama.cpp/discussions/4167 (M3 Pro measured) · github.com/XiongjieDai/GPU-Benchmarks-on-LLM-Inference · en.wikipedia.org/wiki/Apple_M3

Pure-Go / embedding: github.com/gomlx/gomlx · github.com/gomlx/gemma · github.com/nlpodyssey/spago · github.com/tmc/go-llama2 · github.com/gotzmann/llama.go · github.com/shota3506/onnxruntime-purego · medium.com/@vladimirvivien/building-gemma-4-local-powered-llm-apps-with-go-and-yzma-6bc43d48ee4e

Licensing: huggingface.co/Qwen/Qwen2.5-7B/blob/main/LICENSE · github.com/LG-AI-EXAONE/EXAONE-3.0/blob/main/LICENSE · ai.google.dev/gemma/terms · llama.com/llama3_1/license/ · huggingface.co/upstage/SOLAR-10.7B-Instruct-v1.0 · techcrunch.com/2025/03/14/open-ai-model-licenses-often-carry-concerning-restrictions/
