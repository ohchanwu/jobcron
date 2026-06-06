# job-scraper — 신입 IT Job Briefing

A calm daily job-posting briefing for Korean new-grad (신입) IT job seekers.

`job-scraper` is a single binary that opens a local web app. Click **스크랩 시작**
and it scrapes Korean job boards — the aggregators [점핏 (Jumpit)](https://jumpit.saramin.co.kr),
[랠릿 (Rallit)](https://www.rallit.com), [데모데이](https://demoday.co.kr), and
[그리팅 (Greeting)](https://greetinghr.com), plus the company career boards
[당근 (Daangn)](https://team.daangn.com), 크래프톤, 몰로코, and 센드버드 (via Greenhouse) —
scores every new-grad IT posting against your profile, and shows a one-page daily
briefing — each match explained, no notifications, no setup, no account. (워크넷 also
turns on with a free government API key — see *Usage*.)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/dashboard-dark.png">
  <img src="docs/dashboard.png" alt="The daily briefing — warm cream in light mode, warm charcoal in dark mode, with gold score numerals and chip-style breakdowns">
</picture>

## Why

신입 IT job hunting in Korea means checking a dozen portals every day while also
doing algorithm prep, resume writing, and portfolio work. Existing tools amplify
the stress — notification spam, spreadsheet exports, ATS-style scoring that feels
like surveillance.

job-scraper is built for the *emotional layer*: a calm morning ritual that does
the daily grind for you. Warm colors, encouraging copy, and zero-match days that
say "천천히 가도 괜찮아요" instead of showing empty-state shame.

## What it does

- **Scrapes several Korean job boards** — 점핏, 랠릿, 데모데이, 그리팅, and the company
  boards 당근·크래프톤·몰로코·센드버드 (plus 워크넷 with a key) — for 신입 IT postings, one
  polite request per second, robots.txt respected. Cross-source duplicates collapse
  onto a single card.
- **Scores each posting** against a structured profile you fill in once: tech
  stack, career level, location, salary floor, and dealbreaker keywords — each
  category's weight adjustable.
- **Explains every score** — `React +20 · 신입 +25 · 서울 +15` — so you can see the
  algorithm working *for* you, not *on* you.
- **Keeps what matters in reach** — a 관심 공고 archive of everything ever scraped,
  북마크 for the ones you're chasing, and a 숨긴 공고 list for the ones you're not.
  Low-scoring postings fold away below a minimum-score line you set.
- **Filters by source** right on the briefing, so you can read one portal at a time —
  and the filter (and the 관심 공고 점수순/날짜순 sort) stick across pages and visits.
- **Streams the scrape live**, so the slow part becomes the interesting part.
- **Optional AI scoring (bring your own key).** Add an Anthropic or OpenAI key and
  the briefing gains evidence-cited adjustments — each one backed by a real quote
  from the posting, with a daily token budget you control. Entirely optional; with
  no key the app scores exactly as before. See *AI scoring* below.
- Runs entirely on your machine. No server, no account, no telemetry.

## Install

Download the binary for your platform from the
[latest release](https://github.com/ohchanwu/job-scraper/releases/latest), unpack
it, and run it. The app opens at <http://localhost:7777>.

**macOS (Apple Silicon)**

```sh
curl -L https://github.com/ohchanwu/job-scraper/releases/latest/download/job-scraper_darwin_arm64.tar.gz | tar xz
./job-scraper
```

**macOS (Intel)** — use `job-scraper_darwin_amd64.tar.gz`.

**Linux (x86-64)**

```sh
curl -L https://github.com/ohchanwu/job-scraper/releases/latest/download/job-scraper_linux_amd64.tar.gz | tar xz
./job-scraper
```

**Windows** — download `job-scraper_windows_amd64.zip`, unzip, run `job-scraper.exe`.

### First-run notes

- **macOS Gatekeeper** may block an unsigned binary. Right-click the file → Open,
  or run `xattr -d com.apple.quarantine ./job-scraper`.
- **Windows SmartScreen**: choose **More info → Run anyway**.

## Usage

1. On first run you land on the profile form. Fill in your stacks, location, and
   any dealbreaker keywords, then save.
2. Click **스크랩 시작** and watch the scrape stream in.
3. Read your briefing — postings sorted by fit, each score broken down.

Run it once a day. That is the whole ritual.

Flags: `--port` (default `7777`), `--no-open` (do not open a browser),
`--db` (override the database path), `--worknet-api-key` (enable the 워크넷
source — a free key from [data.go.kr](https://www.data.go.kr); also read from
`JOBSCRAPER_WORKNET_KEY`), `--version`.

Your data lives in one SQLite file under the OS config directory
(`~/Library/Application Support/job-scraper/` on macOS, `~/.config/job-scraper/`
on Linux, `%APPDATA%\job-scraper\` on Windows).

## Known limitations

- **Keyword matching is token-exact.** "개발" does not match "개발자", and the
  matcher cannot distinguish "야근 없음" from "야근" — enter short, plain
  root-form keywords for your dealbreaker list.
- **The briefing is today's postings.** The front page shows what was first seen
  today — the daily ritual. Everything ever scraped stays in 관심 공고 (sortable by
  date or by fit), so nothing is lost; it just isn't shouting at you each morning.
- **New-grad IT only.** Sources are queried with their 신입 / entry filters; this
  is not a general job search.
- No notifications, no background scheduling, no résumé parsing — by design.

## AI scoring (optional, v2.0, bring your own key)

Off by default. On the profile form, open **AI 분석 (선택)**, pick a provider
(Anthropic or OpenAI), paste your own API key, and fill in a few free-text goals
(what work you like, what you want to avoid). Your key is stored only in a local
0600-permission file next to the database — never uploaded, never shown again
after you save it.

With AI on, a scrape automatically rates the briefing's new postings, and each can
carry an **AI 분석** chip you click to see the exact quote from the posting that
justifies the adjustment — no quote, no adjustment. A per-page **재평가** button
re-rates the postings you're looking at (for example after you change your goals,
or to analyze more than one scrape covered), and a daily token budget (which you
set) keeps spend bounded.

This is the **v2.0** line and ships as a `-alpha` prerelease while the live
provider paths get more real-world mileage. Everything else in the app works
identically whether or not AI is configured.

## Build from source

```sh
git clone https://github.com/ohchanwu/job-scraper
cd job-scraper
go build ./cmd/job-scraper
```

Requires Go 1.26+. Pure Go — no CGO, no external runtime, no database to install.

## Contributing

Issues and pull requests are welcome, especially from other Korean dev job
seekers. Parked future ideas live in [feature-ideas.md](feature-ideas.md).

## License

MIT — see [LICENSE](LICENSE).
