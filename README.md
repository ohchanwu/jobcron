# jobcron — 신입 IT Job Briefing

_[한국어로 보기 🇰🇷](README.ko.md)_

A calm daily job-posting briefing for Korean new-grad (신입) IT job seekers.
The primary full app will be available at `jobcron.app`: set your profile once,
then let Jobcron gather and rank the day's openings into one calm page.

Jobcron scrapes Korean job boards — the aggregators [점핏 (Jumpit)](https://jumpit.saramin.co.kr),
[랠릿 (Rallit)](https://www.rallit.com), [데모데이](https://demoday.co.kr), and
[그리팅 (Greeting)](https://greetinghr.com), plus the company career boards
[당근 (Daangn)](https://team.daangn.com), 크래프톤, 몰로코, and 센드버드 (via Greenhouse) —
scores every new-grad IT posting against your profile, and shows a one-page daily
briefing with every match explained. (워크넷 also turns on with a free government
API key — see *Usage*.)

> **Deployment status:** The read-only demo is live at
> [demo.jobcron.app](https://demo.jobcron.app). The full production app at
> `jobcron.app` is not publicly available yet; its deployment configuration is
> ready, and launch is coming soon.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/dashboard-dark.png">
  <img src="docs/assets/screenshots/dashboard.png" alt="Score-sorted all-postings page with source filters and AI evaluation chips">
</picture>

## Why

신입 IT job hunting in Korea means checking a dozen portals every day while also
doing algorithm prep, resume writing, and portfolio work. Existing tools amplify
the stress — notification spam, spreadsheet exports, ATS-style scoring that feels
like surveillance.

jobcron is built for the *emotional layer*: a calm morning ritual that does
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
- **Keeps what matters in reach** — a 전체 공고 archive of everything ever scraped,
  북마크 for the ones you're chasing, and a 숨긴 공고 list for the ones you're not.
  Low-scoring postings fold away below a minimum-score line you set.
- **Filters by source** right on the briefing, so you can read one portal at a time —
  and the filter (and the 전체 공고 점수순/날짜순 sort) stick across pages and visits.
- **Streams the scrape live**, so the slow part becomes the interesting part.
- **Optional AI scoring (bring your own key).** Add an Anthropic API key and
  the briefing gains evidence-cited adjustments — each one backed by a real quote
  from the posting, with a daily token budget you control. Entirely optional; with
  no key the app scores exactly as before. See *AI scoring* below.
- A read-only web demo is live now, and the full `jobcron.app` experience is
  coming soon. Local operation remains available for contributors and self-hosters.

## Usage

1. On first run you land on the profile form. Fill in your stacks, location, and
   any dealbreaker keywords, then save.
2. Click **스크랩 시작** and watch the scrape stream in.
3. Read your briefing — postings sorted by fit, each score broken down.

Run it once a day. That is the whole ritual.

## Known limitations

- **Matching is AI-assisted when AI is configured.** The first AI layer reads
  career and education requirements from the posting text before scoring,
  rather than relying only on exact tokens; if AI is unavailable, deterministic
  rules take over. Explicit dealbreaker keywords remain literal filters:
  "개발" does not match "개발자", while "야근" also catches "야근 없음".
- **The briefing is today's postings.** The front page shows what was first seen
  today — the daily ritual. Everything ever scraped stays in 전체 공고 (sortable by
  date or by fit), so nothing is lost; it just isn't shouting at you each morning.
- **New-grad IT only.** Sources are queried with their 신입 / entry filters; this
  is not a general job search.
- No notifications, no background scheduling, no résumé parsing — by design.

## AI scoring (optional, v2.0, bring your own key)

Off by default. On the profile form, open **AI 분석 (선택)**, select **Anthropic**
as the provider, paste your own API key, and fill in a few free-text goals
(what work you like, what you want to avoid). Your key is stored only in a local
0600-permission file next to the database — never uploaded, never shown again
after you save it.

With AI on, a first layer reads each posting's career range, new-grad eligibility,
and education requirements before the regular score is calculated. A second layer
compares the posting with your free-text goals, so each posting can carry an
**AI 분석** chip you click to see the exact quote that justifies the adjustment —
no quote, no adjustment. A per-page **AI 평가** button re-rates the postings you're
looking at (for example after you change your goals, or to analyze more than one
scrape covered), and a daily token budget (which you set) keeps spend bounded.

This is the **v2.0** line and ships as a `-alpha` prerelease while the live AI
path gets more real-world mileage. Everything else in the app works identically
whether or not AI is configured.

## Advanced local use

Most users should use `jobcron.app` once it launches. Local binaries and source
builds remain available for contributors and self-hosters who want to run the
writable app themselves.

### Install a release binary

Download the binary for your platform from the
[latest release](https://github.com/ohchanwu/jobcron/releases/latest), unpack
it, and run it. The app opens at <http://localhost:7777>.

**macOS (Apple Silicon)**

```sh
curl -L https://github.com/ohchanwu/jobcron/releases/latest/download/jobcron_darwin_arm64.tar.gz | tar xz
./jobcron
```

**macOS (Intel)** — use `jobcron_darwin_amd64.tar.gz`.

**Linux (x86-64)**

```sh
curl -L https://github.com/ohchanwu/jobcron/releases/latest/download/jobcron_linux_amd64.tar.gz | tar xz
./jobcron
```

**Windows** — download `jobcron_windows_amd64.zip`, unzip, run `jobcron.exe`.

### First-run notes

- **macOS Gatekeeper** may block an unsigned binary. Right-click the file → Open,
  or run `xattr -d com.apple.quarantine ./jobcron`.
- **Windows SmartScreen**: choose **More info → Run anyway**.
- **Upgrading from `job-scraper`:** fully stop every old `job-scraper` process
  before the first normal `jobcron` launch. That launch atomically renames the
  whole application-data directory to `jobcron`, keeping the database, SQLite
  sidecar files, backups, and AI keys together.
- If both the old and new application-data directories exist, `jobcron` refuses
  to start and modifies neither. With both apps stopped, back up both directories,
  determine which contains the current data, and move the other aside so only the
  intended directory remains before retrying. To roll back, stop `jobcron`, confirm
  no process has the database open, rename the `jobcron` directory back to
  `job-scraper`, and then start the old binary.

Flags: `--port` (default `7777`), `--no-open` (do not open a browser),
`--db` (override the database path), `--worknet-api-key` (enable the 워크넷
source — a free key from [data.go.kr](https://www.data.go.kr); also read from
`JOBCRON_WORKNET_KEY`), `--version`.

Your data lives in one SQLite file under the OS config directory
(`~/Library/Application Support/jobcron/` on macOS, `~/.config/jobcron/`
on Linux, `%APPDATA%\jobcron\` on Windows).

### Interactive localhost preview

Run `scripts/preview-interactive.sh [port]` (default `17778`) to try the normal,
writable app at `http://127.0.0.1:17778` without touching your usual app data.
The launcher takes an atomic lock scoped to your user and HTTP port, refuses an
unrelated listener, and starts the shared `jobcron-local` PostgreSQL service. It
then creates a unique disposable database, private state directory, and
encryption key. The preview runs in non-production mode with scheduling disabled.

On exit it drops only that preview database and removes its private state; it
never runs Compose `down` against the shared service. Set
`JOBCRON_PREVIEW_KEEP=1` to retain both and print their exact manual cleanup
commands. Use those commands instead of bringing down the shared Compose project.

### Build from source

```sh
git clone https://github.com/ohchanwu/jobcron
cd jobcron
go build ./cmd/jobcron
```

Requires Go 1.26+. Running the JavaScript lifecycle tests also requires the
pinned CI version, Node.js 22.15.1 LTS. The shipped application remains pure Go
with no CGO or Node.js runtime dependency.

### Local PostgreSQL development

The managed local lifecycle uses Docker project `jobcron-local`, PostgreSQL 18
at `postgres://postgres@127.0.0.1:55432/jobcron_dev?sslmode=disable`, and the
named volume `jobcron-postgres18-cluster`. Docker Engine, the `docker compose`
plugin, and a running Docker daemon are required. The preview starts this service
automatically; the same managed path becomes ordinary local startup after the
Slice 4 verified SQLite import activates the final writable cutover.

```sh
scripts/preview-interactive.sh
```

An explicit `DATABASE_URL` bypasses managed Docker startup and must point to a
database with exactly one existing user. During Slice 3, an ordinary launch with
no URL still uses the legacy SQLite path, and `--db` remains available. Do not
treat PostgreSQL as the final writable cutover until Slice 4's importer has
verified the SQLite migration.

Compose health or container metadata alone is not enough: startup also requires
real TCP reachability at `127.0.0.1:55432`. Failures preserve containers and
volumes and print `ps` and `logs postgres` diagnostics; startup never removes
state automatically. Ordinary app shutdown also leaves PostgreSQL running.
Explicit stop, reset, and narrowly scoped malformed-container recovery commands
live in [deploy/local/README.md](deploy/local/README.md).

## Contributing

Issues and pull requests are welcome, especially from other Korean dev job
seekers. Parked future ideas live in [feature-ideas.md](docs/product/feature-ideas.md).

## License

MIT — see [LICENSE](LICENSE).
