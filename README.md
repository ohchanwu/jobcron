# job-scraper — 신입 IT Job Briefing

A calm daily job-posting briefing for Korean new-grad (신입) IT job seekers.

`job-scraper` is a single binary that opens a local web app. Click **스크랩 시작**
and it scrapes [점핏 (Jumpit)](https://jumpit.saramin.co.kr), scores every new-grad
IT posting against your profile, and shows a one-page daily briefing — each match
explained, no notifications, no setup, no account.

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

- **Scrapes 점핏** for 신입 IT postings — one polite request per second, robots.txt
  respected.
- **Scores each posting** against a structured profile you fill in once: tech
  stack, career level, location, salary floor, and dealbreaker keywords.
- **Explains every score** — `React +20 · 신입 +25 · 서울 +15` — so you can see the
  algorithm working *for* you, not *on* you.
- **Streams the scrape live**, so the slow part becomes the interesting part.
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
`--db` (override the database path), `--version`.

Your data lives in one SQLite file under the OS config directory
(`~/Library/Application Support/job-scraper/` on macOS, `~/.config/job-scraper/`
on Linux, `%APPDATA%\job-scraper\` on Windows).

## Known limitations (v1)

- **One source.** v1 scrapes 점핏 only; more portals are planned.
- **Keyword matching is token-exact.** "개발" does not match "개발자", and the
  matcher cannot distinguish "야근 없음" from "야근" — enter short, plain
  root-form keywords for your dealbreaker list.
- **Today-only briefing.** The dashboard shows postings first seen today — it is
  a daily ritual, not a searchable archive.
- No notifications, no background scheduling, no résumé parsing — by design.
  (Optional bring-your-own-key AI scoring is the in-progress v2.0 line; its
  foundation has landed in the code but is not yet user-facing.)

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
