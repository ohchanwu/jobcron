# 데모데이 vs 랠릿 vs 점핏: A Comparative Report for the 신입 (New-Grad) IT Job Seeker

## Executive summary

데모데이 (Demoday, demoday.co.kr) lists far more jobs than 랠릿 (Rallit) and 점핏 (Jumpit) not because it is a bigger or better recruiting product, but because it is a fundamentally *different kind of site*: a broad 창업 정보 플랫폼 (startup-information platform) running since 2012, where job posting is one free feature sitting beside investment data, company rankings, business-model analysis, funding insights, and a community [0][1][2][3][17][18][19][20]. By contrast, 점핏 is Saramin's developer-only recruiting platform with a paid, success-fee business model (7% of the hire's annual salary) [8][9][10][11][12], and 랠릿 is 인프랩's (Inflab's) IT-talent platform — the company behind the 인프런 (Inflearn) e-learning service — scoped to IT roles and marketing itself around postings from "companies vetted within the industry" [14][15][16]. The verified evidence supports two of the three drivers of 데모데이's higher count cleanly (free-to-post vs paid, and all-job-functions vs developer-only) and the third (curation/staleness gap) partially. The practical consequence for a backend-leaning 신입 (e.g., Go / MySQL / vanilla JavaScript): 데모데이's bigger number is mostly noise to scan, not signal to chase — the dedicated tech platforms (점핏, 랠릿, plus 원티드) give a higher hit-rate per minute spent.

A note on scope and confidence: most claims below are **high confidence**, backed by primary sources (the platforms' own pages and legally-required footers) plus independent corroboration. Where a sub-claim rests on a single secondary source, or where verification *refuted* a working assumption, I flag it inline. The one area I could not verify directly is 랠릿's exact business model — see the caveats section.

---

## 1. Platform identity and business model

### 데모데이 (Demoday) — a startup-ecosystem portal, not a job board

데모데이 self-identifies as a **창업 정보 플랫폼 (startup-information platform)** for "창업자와 예비창업자" (founders and prospective entrepreneurs), spanning startups, SMEs (small and medium enterprises), sole proprietorships, and small merchants [0]. Its own verbatim self-description reads: "데모데이는 창업자와 예비창업자를 위한 창업 정보 플랫폼입니다" [0][6]. Across its pages it brands itself as "데모데이 - 스타트업 포털 서비스" (Startup Portal Service) and "대한민국 대표 스타트업 플랫폼" (Korea's leading startup platform) [0][18][19]. Confidence: **high** — this is a self-identification claim, and the platform's own pages are the correct authority for it.

The site has operated since **2012**, run by **㈜데모데이 (Demoday Inc.)** [1][7][17][20]. Its footer reads "Copyright ©2012 Demoday Inc." and the independent Korean venture/VC database THE VC (thevc.kr) records founding date 2012-09-03, with CEO **나승국 (Na Seung-guk)**, headquartered in Gangnam, Seoul [1][20]. One verified nuance (refinement, not a contradiction): the corporate entity was originally registered as "엔젤들" (Angels) on 2012-09-03 and renamed to "데모데이" on 2013-08-05 — so the *entity* dates to 2012 but the *brand name* to 2013 [1][7]. Confidence: **high** (corporate founding date corroborated by both the site and an independent registry-grade database; the exact street address and "current CEO" status rest on the single THE VC source [20]).

Crucially, **job posting (채용) is one feature among eight.** 데모데이's top-level navigation spans 정보 (Information), 채용 (Recruiting), 평판 (Reputation), BM분석 (Business-Model Analysis), 펀딩 (Funding), 스타트업 (Startup Database), 창업도구 (Founder Tools), and 커뮤니티 (Community) [3][5]. Beyond these, it offers company reputation analysis ("평판분석"), business-model analysis ("BM분석"), funding/investment case studies, company rankings ("스타트업 Top 5"), and startup calculators (salary/severance) [2][18][19]. Its own about page enumerates: startup information (company data, investment status, job openings), an investor network (VCs, accelerators, angels), funding insights, and community [18][19]. The platform reports roughly **15,000 registered startups** [0][2][3]. A real BM분석 article (on FastFive) confirmed that recruiting appears only as supplementary material ("지금 채용 중") at the end of substantive editorial content — concrete evidence that recruiting is secondary, not core [2]. Confidence: **high**.

On **monetization**: the strong, directly-verified part of the hypothesis is that recruiting is a *secondary feature*, not the platform's core paid product [2][3][5]. The "job posting is free" half could **not** be confirmed with direct pricing data — verification explicitly flagged this as an inference consistent with the evidence (feature prominence, portal framing) rather than a proven fact [2]. Confidence on "recruiting is secondary": **high**. Confidence on "posting is free": **medium** (inferred, not price-verified).

### 점핏 (Jumpit) — Saramin's developer-only platform, paid success fee

점핏 is operated by **(주)사람인 (Saramin Inc.)**, the established Korean recruiting company [8][12]. The legally-required footer names the operator, CEO **황현순 (Hwang Hyun-soon)** (ex-Kiwoom Securities CEO, appointed March 2024, independently corroborated by 한국경제 and 이코노미스트), business registration 113-86-00917, and a Seoul (강서구 마곡동) address [8]. Saramin launched 점핏 around **March 2021** [9][11][12]. Notably, 점핏's *employer service* is being merged into Saramin's corporate recruiting service (사람인 채용 센터) as of a "6월 28일" announcement [8]. Confidence: **high** (ownership facts from the company's own footer plus business-press corroboration of the CEO).

점핏 **positions itself** as "국내 유일 개발자 전문 채용 플랫폼" (Korea's only developer-specialized recruiting platform), built around matching by job function and tech stack [9]. The positions page is titled "점핏 | 개발 직무 탐색" (Developer Job Exploration), and its only filters are 기술스택 (tech stack), 경력 (career/experience level), 지역 (location), and 태그 (tags) — **no design, marketing, sales, or planning tracks** [11][13]. Saramin's own 2021 launch press release describes it as enabling postings and applications "based on tech stack (the programming languages and frameworks used in IT development)" [9][13]. One honest qualifier: the "국내 유일 (only)" boast is marketing language and is arguably false as an objective statement (랠릿 and 프로그래머스 are competing developer platforms) — but the verified claim is precisely that 점핏 *says this about itself*, which it does [9]. Confidence: **high** for self-positioning and developer-only scope.

**Business model: paid B2B, success-based.** Employers post for free until a hire is made, then pay a placement fee of **7% of the hired candidate's annual salary (VAT excluded)** [10]. This is confirmed by 점핏's official "기업 서비스 이용 가이드" PDF ("채용확정형 상품," "채용확정된 자 연봉의 7%") and the official help center [10]. Confidence: **high** (the "부가세 별도 / VAT-excluded" detail is corroborated by help-center summaries rather than the PDF text directly, but the 7%-on-hire mechanism is solid).

### 랠릿 (Rallit) — 인프랩's IT-talent platform, tied to a developer-education base

랠릿 is operated by **(주)인프랩 (Inflab Inc.)** — the same company behind the online learning platform **인프런 (Inflearn)** [15][16]. This is bound by a hard technical fingerprint (the mobile app package IDs are com.inflab.rallit on Google Play and Inflab's namespace on Apple's App Store) plus launch press and 6+ neutral company databases (THE VC, RocketPunch, JobKorea, etc.) [15]. 랠릿 **officially launched on 2022-01-27** per Inflab's CEO's own blog post; press coverage dated 2022-02-17 reflects the standard soft-launch-vs-press-announcement gap, not a contradiction [16]. Confidence: **high**.

랠릿 **positions itself as an IT-talent recruitment platform** ("IT 인재 채용 플랫폼"), not a general all-functions job board — the page title is literally "IT 인재 채용 플랫폼 - 랠릿" and the official Inflab description scopes it to "개발, 디자인, 기획 등 필요한 IT 전문 인재" (dev, design, planning — IT-specialized talent) [14]. So it is **IT-professional-scoped (broader than 점핏's developer-only, narrower than 데모데이's all-functions)** [14]. Two verification corrections worth flagging:

- **Refuted/corrected evidence:** The site's *actual* tagline is "업계에서 검증된 회사들의 채용 공고를 랠릿에서 만나보세요" (Meet job postings from companies *vetted within the industry*) — **not** the IT-scoping tagline a working claim had attributed to it. The IT-scoping comes from the page title/branding, not the tagline [14]. The "vetted companies" tagline is in fact the basis for the report's working premise that 랠릿 markets itself around industry-vetted postings — confirmed [14].
- **Refuted working claims:** Three claims asserting 랠릿's specifics were **refuted in verification** and should not be relied on: (a) that 랠릿's visible categories are *exclusively* technical/IT roles (refuted 0-3 — it includes 기획/planning and 디자인/design, so "exclusively technical" overstates it) [14]; and (b)/(c) two claims that 랠릿 runs a paid B2B model via a separate business portal at business.rallit.com (both refuted 0-3). The B2B-via-business-portal mechanism did **not** survive verification — see caveats. A fourth claim narrowly calling 랠릿 an "IT 전문 커리어 플랫폼" was refuted 1-2 on quote grounds, though the broader IT-scoping is independently confirmed [14].

Confidence: **high** on operator/launch/IT-scoping; **low** on business-model specifics (the paid-B2B-portal claim was refuted, leaving 랠릿's exact monetization *unverified* here).

---

## 2. The core question — why 데모데이 lists so many more jobs

The hypothesis was that 데모데이's higher count compounds from three factors. Here is the evidence for each.

**(a) Free posting (no cost discipline) vs paid B2B — partially verified, directionally strong.**
점핏 is confirmed paid: it charges 7% of the hire's salary on success [10]. 랠릿's exact model is *unverified* (the paid-B2B-portal claim was refuted) [14], so the clean contrast is really "점핏 is paid" vs "데모데이 recruiting is a secondary, very likely free feature." On the 데모데이 side, "free" is an inference from feature-prominence, not price-verified [2]. The economic logic holds — paid platforms impose cost discipline that suppresses count, and a free secondary feature does not — but treat the "free vs paid" framing as **directionally strong, not fully proven**. Confidence: **medium**.

**(b) All job functions vs developer-only scope — verified, high confidence.**
This is the cleanest driver. 데모데이's job board carries software/DevOps/data engineering, web design, performance/content marketing, MD/growth-commerce, sales, HR, and finance/accounting roles — verified against individual posting pages (e.g., a real-time data-pipeline engineer with CDC+Kafka work, a cosmetics brand manager, an iOS developer, a sales leader, a junior HR role) [4]. A search summary describes the board as carrying "개발, 디자인, 마케팅, 영업 등 다양한 직군" (dev/design/marketing/sales and other functions) [4][19]. 점핏, by direct contrast, is developer-only (~20 developer categories, no non-dev tracks) [11]. So 데모데이's count draws from a far larger pool of job functions. Confidence: **high**.

**(c) Curation gap (anyone registered can post + stale postings never pruned) vs vetted/tech-filtered — partially supported.**
The *vetted/tech-filtered* side of the contrast is solid: 랠릿 explicitly markets industry-*vetted* companies [14], and 점핏 is tech-stack-filtered and developer-scoped [11][13]. The 데모데이 side — that "anyone registered can post" and "stale postings are never pruned" — was **not** directly verified in the surviving claims. It is plausible given the portal model and ~15,000 registered startups [0][2], but I have no verified evidence on 데모데이's posting-eligibility rules or pruning/expiry behavior. Confidence: **low-to-medium** (the contrast platforms are verified; 데모데이's curation/staleness behavior is inferred, not measured).

**Bottom line on the core question:** The count gap is real and is driven primarily by **two structural differences that are well-verified** — 데모데이 covers all job functions (vs 점핏's developer-only) [4][11], and recruiting on 데모데이 is a free secondary feature on a 15,000-company portal rather than a cost-disciplined paid product [2][10]. The third factor (curation/staleness) is consistent with the model but not directly verified.

---

## 3. Listing quality and signal-to-noise

For a candidate targeting tech-product companies with a backend-ish profile (e.g., Go / MySQL / vanilla JavaScript), the composition differences matter more than the raw count:

- **데모데이** skews toward early-stage startups and spans every job function [4]. For a backend 신입, this means a large fraction of any given page is *irrelevant by function* (marketing, sales, MD, HR, finance, design) [4], and the dev roles that do appear are mixed in with non-dev noise. There is no tech-stack filter to isolate Go/MySQL roles — the platform's primary filters are organized around startups and funding, not languages/frameworks [3][5]. The startup skew also means more very-early companies. **Signal-to-noise: low for a backend 신입** — you scan a lot to find a little.
- **점핏** is purpose-built for exactly this candidate: 기술스택 (tech stack) is a first-class filter, alongside 경력 (career level) and 지역 (location) [11][13]. A 신입 can filter to new-grad developer roles and tech-stack matches directly. **Signal-to-noise: high** — though the absolute count is smaller (the design context notes the 신입 universe is on the order of ~57 postings).
- **랠릿** is IT-scoped (dev + design + planning) and markets industry-*vetted* companies [14], so the noise floor is lower than 데모데이's even if it carries some non-developer IT roles. **Signal-to-noise: medium-to-high**, with the caveat that its exact filtering and freshness behavior weren't verified here.

The practical read: 데모데이's bigger number does not translate into more *relevant* backend opportunities per unit of scanning time. A higher count of mostly-non-dev, mostly-early-stage, unfiltered-by-stack listings is a weaker funnel than a smaller count of developer-scoped, stack-filterable, new-grad-tagged listings.

---

## 4. Where 원티드 (Wanted) fits

I was **not** able to verify any claims about 원티드 (Wanted, wanted.co.kr) in this research pass — none of the 21 surviving verified claims concern it. So I cannot confirm or refute, from verified evidence here, the working premise that 원티드 is where mid-stage tech-product hiring concentrates. This is an **open gap** flagged in the caveats and open questions below. (What I can say structurally, without a verified citation: 원티드 is widely understood in the Korean market as a broad IT/professional hiring platform known for its referral-and-reward model, positioned between 점핏's developer specialization and the general boards — but treat that as **unverified context**, not a finding from this report's evidence base.)

---

## 5. Bottom line — how a 신입 dev should weight time

**데모데이's bigger number is mostly noise to scan, not a reason to spend more time there.** The size advantage comes from covering all job functions and from being a free secondary feature on a startup portal [2][4][10] — neither of which makes it a *better* funnel for a backend-leaning new grad. Concretely:

1. **Spend your primary time on 점핏.** It is purpose-built for your exact case: developer-only scope, tech-stack filtering for Go/MySQL, and a 경력 (career) filter to isolate 신입 roles [11][13]. Highest signal-per-minute, even though the absolute count is small.
2. **Make 랠릿 your secondary.** IT-scoped and industry-vetted, so lower noise than a general portal; it adds dev roles 점핏 may not carry [14]. (Verify its filters yourself, since its mechanics weren't confirmed here.)
3. **Check 원티드** as a likely third for tech-product roles — but verify its fit yourself, since this report has no verified evidence on it.
4. **Treat 데모데이 as a periodic scan, not a daily driver.** Its value to a backend 신입 is *breadth of early-stage startups you won't see elsewhere*, not relevance density. Without a tech-stack filter and with most listings being non-dev or very-early-stage [3][4][5], the time cost per relevant hit is high. Scan it occasionally for startup-curious exploration; don't let its larger number pull disproportionate time away from the dedicated tech platforms.

In one line: **count ≠ relevance.** A smaller, developer-scoped, stack-filterable list beats a larger, all-functions, unfiltered one for a 신입 with a backend profile.

---

## Caveats and uncertainties

- **데모데이 "free posting" is inferred, not price-verified.** Verification confirmed recruiting is a *secondary* feature but could not confirm posting is free with pricing data [2]. Treat the free-vs-paid contrast as directionally strong, not proven.
- **랠릿's business model is unverified.** Three working claims that 랠릿 runs a paid B2B model via business.rallit.com were **refuted (0-3)** in verification [14]. This report therefore makes *no* verified claim about how 랠릿 monetizes. The "industry-vetted companies" positioning *is* verified [14]; the monetization mechanism is not.
- **데모데이's curation/staleness is inferred.** No surviving claim verifies that "anyone registered can post" or that stale postings are never pruned. The vetted/filtered contrast (랠릿/점핏) is verified [11][13][14]; 데모데이's posting rules and freshness are not.
- **원티드 is entirely unverified here.** No verified claim covers it. Section 4 is a gap, not a finding.
- **Time-sensitivity:** 점핏's employer service is being merged into 사람인 채용 센터 (a "6월 28일" announcement) [8] — this could change 점핏's posting flow, count, or even brand independence in the near term. Founding dates are immutable; CEO and business-model details are current as of the June 2026 verification pass but can drift. The 7% fee and "vetted companies" positioning are current snapshots.
- **Single-source / lower-quality flags:** 데모데이's exact street address and current-CEO status rest on the single secondary source THE VC [20]. The "VAT-excluded" detail on 점핏's fee is corroborated by help-center summaries rather than the primary PDF [10].

## Open questions

1. **What is 랠릿's actual business model?** The paid-B2B-portal claim was refuted. Does it charge a success fee like 점핏, a subscription, or something else? (Needs primary-source pricing verification.)
2. **Does 데모데이 actually let any registered startup self-post for free, and does it prune stale listings?** Quantifying posting eligibility and median posting age would directly confirm or kill driver (c) of the count hypothesis.
3. **Where does 원티드 actually sit?** Is mid-stage tech-product hiring really concentrated there relative to 점핏/랠릿, and what is its developer-role volume and freshness for a 신입 backend profile?
4. **How many of 데모데이's dev listings are genuinely new-grad-eligible and tech-product (not agency/SI/early-stage churn)?** A composition audit (dev vs non-dev share, 신입 share, stale share) would turn the "mostly noise" verdict from reasoned inference into measured fact.

---

## Sources

1. 데모데이 homepage — https://demoday.co.kr/ [0][1][2][3]
2. 데모데이 (main subdomain, redirects to homepage) — https://main.demoday.co.kr/ [6][7]
3. 데모데이 recruiting board — https://demoday.co.kr/recruits [4][5]
4. 데모데이 weekly company rankings — https://demoday.co.kr/companies/rank/weekly [3]
5. 데모데이 about page — https://demoday.co.kr/about [17][18][19]
6. THE VC — 데모데이 company profile — https://thevc.kr/demoday [1][7][20]
7. 점핏 business/employer site — https://biz.jumpit.saramin.co.kr/ [8][9][10]
8. 점핏 positions page — https://jumpit.saramin.co.kr/positions [11][12][13]
9. 점핏 2025 recruitment report — https://jumpit.saramin.co.kr/report/2025/recruit [9]
10. Saramin 점핏 launch press release (March 2021) — https://www.saramin.co.kr/zf_user/help/live/view?idx=108164 [9][12][13]
11. 점핏 employer service guide PDF (2021-07) — https://cdn.jumpit.co.kr/jumpit/jumpit_biz_guide_202107_v2.pdf [10]
12. 한국경제 — Saramin CEO 황현순 appointment — https://www.hankyung.com/article/2024032045851 [8]
13. 한국경제 매거진 — 점핏 tech-stack design — https://magazine.hankyung.com/business/article/202402053693b [13]
14. 랠릿 homepage — https://www.rallit.com/ [14]
15. 랠릿 biz feed post — https://www.rallit.com/feed/2/rallit-biz-230426 [15]
16. 인프런 weekly post (랠릿 launch) — https://www.inflearn.com/pages/weekly-inflearn-42-20220222 [16]
17. ko.wikipedia.org — 점핏 — https://ko.wikipedia.org/wiki/점핏 [11]
18. FN News — 인프랩 launches 랠릿 (2022-02-17) — https://www.fnnews.com/news/202202171518106724 [15][16]

---

## Methodology & provenance

Produced 2026-06-03 by the gstack `deep-research` workflow: 5 search angles → 22 sources fetched → 95 candidate claims → 25 sent to 3-vote adversarial verification → **21 confirmed, 4 refuted** (a claim is killed on a 2-of-3 refute vote). Findings are written only from confirmed claims; single-source or inferred points are flagged inline in the report above.


**Claims that were checked and refuted** (excluded from the findings, listed for transparency):

- Rallit's visible job categories are exclusively technical/IT roles (backend, frontend, DevOps, QA, product manager), supporting the claim that it is developer/tech-scoped rather than covering marketing/sales/non-dev functions like a broad startup portal.  _(vote 0-3; https://www.rallit.com/)_
- Rallit operates a paid B2B posting model: companies post jobs through a separate business portal (business.rallit.com) rather than any registered user posting for free.  _(vote 0-3; https://www.rallit.com/)_
- Posting jobs on 랠릿 is a business (B2B) service: companies register through a separate business portal (business.rallit.com) rather than posting freely from a general user account.  _(vote 0-3; https://www.rallit.com/feed/2/rallit-biz-230426)_
- 랠릿 positions itself specifically as an IT-specialized career platform ("IT 전문 커리어 플랫폼"), confirming the developer/tech scope rather than an all-functions job board.  _(vote 1-2; https://www.inflearn.com/pages/weekly-inflearn-42-20220222)_


*This report is a point-in-time snapshot; platform listing counts, pricing, and org details drift. Re-run the research workflow to refresh.*
