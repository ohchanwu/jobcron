# jobcron — 신입 IT Job Briefing

_[Read this in English 🇬🇧](README.md)_

한국 신입 IT 구직자를 위한 차분한 일일 채용 공고 브리핑입니다.

`jobcron`은 로컬 웹 앱을 여는 단일 바이너리(binary)입니다. **스크랩 시작**을
누르면 네 곳의 한국 채용 출처 — [점핏 (Jumpit)](https://jumpit.saramin.co.kr),
[랠릿 (Rallit)](https://www.rallit.com), [데모데이](https://demoday.co.kr),
[당근 (Daangn)](https://team.daangn.com) — 를 스크랩하고, 모든 신입 IT 공고를
사용자 프로필 기준으로 점수화한 뒤, 한 페이지짜리 일일 브리핑을 보여줍니다. 각 매칭에
대한 설명이 함께 제공되며, 알림도, 설정도, 계정도 없습니다. (다섯 번째 출처인 워크넷은
무료 정부 API 키로 켤 수 있습니다 — *사용법* 참고.)

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/dashboard-dark.png">
  <img src="docs/dashboard.png" alt="The daily briefing — warm cream in light mode, warm charcoal in dark mode, with gold score numerals and chip-style breakdowns">
</picture>

## 왜 만들었나 (Why)

한국에서 신입 IT 구직을 한다는 것은 알고리즘 공부, 이력서 작성, 포트폴리오 작업을
병행하면서도 매일 수십 개의 포털을 확인해야 한다는 뜻입니다. 기존 도구들은 오히려 그
스트레스를 키웁니다 — 알림 폭탄, 스프레드시트 내보내기, 마치 감시처럼 느껴지는
ATS 스타일 점수화 말입니다.

jobcron은 *감정적 측면(emotional layer)*을 위해 만들어졌습니다: 매일의 고된 작업을
대신 처리해 주는, 차분한 아침 의식(ritual)입니다. 따뜻한 색감, 격려하는 문구, 그리고 매칭이
하나도 없는 날에는 텅 빈 화면의 부끄러움을 보여주는 대신 "천천히 가도 괜찮아요"라고
말해 줍니다.

## 무엇을 하나 (What it does)

- **네 곳의 출처를 스크랩합니다** — 점핏, 랠릿, 데모데이, 당근 (키가 있으면 워크넷 추가) —
  신입 IT 공고를 대상으로, 초당 한 건의 정중한 요청으로, robots.txt를 준수하며 가져옵니다.
  여러 출처에 걸친 중복 공고는 하나의 카드로 합쳐집니다.
- **각 공고를 점수화합니다** — 한 번만 입력하면 되는 구조화된 프로필 기준으로 점수를
  매깁니다: 기술 스택, 경력 수준, 지역, 최저 희망 연봉, 그리고 제외
  키워드(dealbreaker keyword, 절대 피하고 싶은 조건). 각 항목의 가중치는 조정할 수 있습니다.
- **모든 점수를 설명합니다** — `React +20 · 신입 +25 · 서울 +15` — 알고리즘이 당신에게
  *불리하게(on)*가 아니라 *유리하게(for)* 작동하고 있음을 직접 확인할 수 있습니다.
- **중요한 것을 가까이 둡니다** — 지금까지 스크랩한 모든 것을 담은 전체 공고 보관함,
  쫓고 있는 공고를 위한 북마크, 그리고 관심 없는 공고를 위한 숨긴 공고 목록을 제공합니다.
  점수가 낮은 공고는 직접 설정한 최저 점수 기준선 아래로 접혀 들어갑니다.
- **출처별로 필터링합니다** — 브리핑 화면에서 바로 가능하므로, 한 번에 한 포털씩 읽을 수
  있습니다.
- **스크랩 과정을 실시간으로 보여줍니다** — 느린 과정이 오히려 흥미로운 과정이 됩니다.
- **선택적 AI 점수화 (당신의 키를 직접 사용).** Anthropic API 키를 추가하면
  브리핑에 증거 인용 기반 조정이 더해집니다 — 각 조정은 공고에서 가져온 실제 인용문으로
  뒷받침되며, 일일 토큰(token) 예산은 당신이 직접 통제합니다. 완전히 선택 사항이며, 키가
  없으면 앱은 이전과 정확히 동일하게 점수를 매깁니다. 아래 *AI 점수화* 참고.
- 전적으로 당신의 기기에서 실행됩니다. 서버도, 계정도, 텔레메트리(telemetry, 사용 데이터
  수집)도 없습니다.

## 설치 (Install)

[최신 릴리스](https://github.com/ohchanwu/jobcron/releases/latest)에서 사용하는
플랫폼용 바이너리를 내려받아 압축을 풀고 실행하세요. 앱은 <http://localhost:7777>에서
열립니다.

**macOS (Apple Silicon)**

```sh
curl -L https://github.com/ohchanwu/jobcron/releases/latest/download/jobcron_darwin_arm64.tar.gz | tar xz
./jobcron
```

**macOS (Intel)** — `jobcron_darwin_amd64.tar.gz`를 사용하세요.

**Linux (x86-64)**

```sh
curl -L https://github.com/ohchanwu/jobcron/releases/latest/download/jobcron_linux_amd64.tar.gz | tar xz
./jobcron
```

**Windows** — `jobcron_windows_amd64.zip`를 내려받아 압축을 풀고
`jobcron.exe`를 실행하세요.

### 첫 실행 시 참고 사항 (First-run notes)

- **macOS Gatekeeper**가 서명되지 않은 바이너리를 차단할 수 있습니다. 파일을 우클릭 →
  Open을 선택하거나, `xattr -d com.apple.quarantine ./jobcron`을 실행하세요.
- **Windows SmartScreen**: **More info → Run anyway**를 선택하세요.

## 사용법 (Usage)

1. 첫 실행 시 프로필 입력 폼으로 이동합니다. 기술 스택, 지역, 그리고 제외
   키워드(절대 피하고 싶은 조건)를 입력한 뒤 저장하세요.
2. **스크랩 시작**을 누르고 스크랩이 실시간으로 흘러 들어오는 것을 지켜보세요.
3. 브리핑을 읽으세요 — 적합도순으로 정렬된 공고와 각 점수의 세부 내역이 표시됩니다.

하루에 한 번 실행하세요. 그것이 의식의 전부입니다.

플래그(flag): `--port` (기본값 `7777`), `--no-open` (브라우저를 열지 않음),
`--db` (데이터베이스 경로 재지정), `--worknet-api-key` (워크넷 출처 활성화 —
[data.go.kr](https://www.data.go.kr)에서 발급하는 무료 키이며, `JOBCRON_WORKNET_KEY`
환경 변수에서도 읽습니다), `--version`.

당신의 데이터는 OS 설정 디렉터리 아래 하나의 SQLite 파일에 저장됩니다
(macOS는 `~/Library/Application Support/jobcron/`, Linux는 `~/.config/jobcron/`,
Windows는 `%APPDATA%\jobcron\`).

## 알려진 한계 (Known limitations)

- **키워드 매칭은 토큰 단위로 정확히 일치해야 합니다.** "개발"은 "개발자"와 매칭되지 않으며,
  매처(matcher)는 "야근 없음"을 "야근"과 구별하지 못합니다 — 제외 키워드 목록에는
  짧고 단순한 어근(root) 형태의 키워드를 입력하세요.
- **브리핑은 오늘 올라온 공고입니다.** 첫 화면은 오늘 처음 발견된 것을 보여줍니다 — 그것이
  일일 의식입니다. 지금까지 스크랩한 모든 것은 전체 공고에 남아 있으므로(날짜순 또는
  적합도순으로 정렬 가능) 잃어버리는 것은 없습니다. 다만 매일 아침 당신에게 소리치지 않을
  뿐입니다.
- **신입 IT 전용입니다.** 출처들은 각자의 신입 / 엔트리(entry) 필터로 조회됩니다. 일반적인
  채용 검색이 아닙니다.
- 알림도, 백그라운드 스케줄링도, 이력서 파싱도 없습니다 — 의도적인 설계입니다.

## AI 점수화 (선택, v2.0, 당신의 키를 직접 사용)

기본적으로 꺼져 있습니다. 프로필 폼에서 **AI 분석 (선택)**을 열고, 제공자로 **Anthropic**을
선택하고, 당신의 API 키를 붙여넣은 뒤, 몇 개의 자유 서술형 목표(어떤 일을
좋아하는지, 무엇을 피하고 싶은지)를 입력하세요. 당신의 키는 데이터베이스 옆의 로컬
0600 권한 파일에만 저장됩니다 — 업로드되지 않으며, 저장한 뒤에는 다시 표시되지 않습니다.

AI를 켜면 공고에 **AI 분석** 칩(chip)이 붙을 수 있으며, 이를 클릭하면 각 점수 조정을
정당화하는 공고 속 정확한 인용문을 볼 수 있습니다 — 인용문이 없으면 조정도 없습니다.
페이지별 **재평가** 버튼은 당신이 보고 있는 공고들을 당신의 목표 기준으로 다시 점수화하며,
(직접 설정하는) 일일 토큰 예산이 지출을 제한선 안에 묶어 둡니다.

이것은 **v2.0** 라인이며, 실제 AI 경로가 현실 세계에서 더 많은 검증을 거치는 동안
`-alpha` 사전 릴리스(prerelease)로 배포됩니다. AI 설정 여부와 무관하게 앱의 나머지 모든
기능은 동일하게 작동합니다.

## 소스에서 빌드하기 (Build from source)

```sh
git clone https://github.com/ohchanwu/jobcron
cd jobcron
go build ./cmd/jobcron
```

Go 1.26 이상이 필요합니다. 순수 Go(Pure Go)로 작성되었습니다 — CGO도, 외부 런타임도,
설치해야 할 데이터베이스도 없습니다.

## 기여하기 (Contributing)

이슈와 풀 리퀘스트(pull request)를 환영하며, 특히 다른 한국 개발자 구직자분들의 기여를
환영합니다. 보류 중인 향후 아이디어는 [feature-ideas.md](feature-ideas.md)에 있습니다.

## 라이선스 (License)

MIT — [LICENSE](LICENSE) 참고.
