# Alpha Pre-Launch Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** Remove the two production launch blockers, make signed-in bookmark removal complete in place, and publish hosted-first root READMEs with verified score-sorted screenshots.

**Architecture:** Production keeps BYOK credentials in the existing filesystem-backed key store, but Compose gives that directory a stable named volume and the runbook reaches private RDS through a localhost SSH tunnel. The bookmarks page keeps the server API unchanged: bookmark.js removes a card only after a successful signed-in DELETE, then emits one generic posting-list-change event so the count, empty state, source filter, and search filter recompute from connected DOM nodes. Documentation makes the hosted app the primary product path while preserving local operation as advanced use; screenshot work happens last against the verified local candidate.

**Tech Stack:** Go 1.26, html/template, vanilla JavaScript, Node.js built-in vm/assert/test modules, Docker Compose, Alpine Linux, PostgreSQL 18, Caddy, Markdown, PNG assets, gstack /browse, and frontend-qa.

**Source specification:** ../specs/260713-alpha-pre-launch-fixes.md

**Human-only follow-up:** ../specs/260713-alpha-launch-human-blocked-steps.md

**Durable product decision:** ../decisions/260714-hosted-first-local-database-convergence.md

## Global Constraints

- Do not edit deploy/demo/, the jobcronDemoBookmarks localStorage contract, or the deployed demo behavior.
- Do not mutate AWS, Docker Hub, DNS, RDS, or the production host. Those actions stay in the human-only specification.
- Keep API keys, passwords, DATABASE_URL values, session secrets, owner identities, private endpoints, and recovery paths out of Git, screenshots, fixtures, logs, and chat.
- The app key directory is exactly /root/.config/jobcron and the named Docker volume is exactly jobcron_config.
- docker compose down preserves the key volume; docker compose down -v is destructive and must never appear as a routine deployment command.
- Keep the production image server-only. Do not add jobcron-user, jobcron-import, repository source, or another shell to the image.
- Owner creation must precede import; both commands must use the same owner email.
- The signed-in bookmark exit reuses .posting.removing and its existing 260 ms hard-removal fallback.
- Demo bookmark behavior remains immediate localStorage-driven hiding with no new transition.
- Dashboard images remain 1440 x 900 PNG files and show 점수순 active with kept postings descending by score.
- Both root READMEs make the upcoming `jobcron.app` the primary full-product path,
  keep `demo.jobcron.app` as the live evaluation path, and group local operation
  under advanced use without deleting any local command or SQLite behavior.
- PostgreSQL convergence for local runs is explicitly post-launch and must not
  expand this implementation's scope.
- Browser verification must use /browse and frontend-qa. Never open the user's default browser.
- Layout-affecting frontend work receives Tier C verification: all signed-in pages, desktop and mobile, light and dark themes, and console-error review.
- Commit locally at the end of each task after the relevant tests and publication gate pass. Never push.
- Angle-bracketed infrastructure values in deployment documentation are intentional public-safe operator placeholders, not missing implementation decisions.

## File Map

| File | Responsibility |
|---|---|
| deploy/production/compose.yaml | Mount the stable jobcron_config volume into the app service. |
| deploy/production/.env.example | Replace the stale concrete image tag with the immutable release-tag shape. |
| deploy/production/README.md | Explain volume durability, destructive commands, and static validation. |
| deploy/production/HUMAN_DEPLOY_GUIDE.md | Give the human an executable image, migration, tunnel, owner, optional import, and key-entry sequence. |
| internal/ai/keys.go | Reference only; its UserConfigDir path, atomic replacement, and 0600 mode are the persistence contract. |
| cmd/jobcron-user/main.go | Reference only; its secure password prompt and create-owner flags remain unchanged. |
| cmd/jobcron-import/main.go | Reference only; its transactional import and password-hash preservation remain unchanged. |
| deploy/production/Dockerfile | Reference only; it must continue to contain only the jobcron server binary. |
| web/bookmarks.html | Supply a hidden signed-in empty-state target when the initial page has cards. |
| internal/server/bookmarks_test.go | Lock the signed-in live empty-state markup and unchanged demo markup. |
| web/bookmark.js | Remove a successfully unbookmarked signed-in card and synchronize page-level state. |
| web/source-filter.js | Reapply active filters after list mutation using only connected cards. |
| web/testdata/bookmark-lifecycle.test.js | Exercise real shipped JavaScript in a zero-package Node fake-DOM harness. |
| web/bookmark_test.go | Run the Node lifecycle harness under go test ./.... |
| web/not-interested.js | Reference only; supplies the existing fade/removal pattern. |
| web/styles.css | Reference only; supplies .posting.removing. |
| docs/assets/screenshots/dashboard.png | 1440 x 900 light score-sorted capture. |
| docs/assets/screenshots/dashboard-dark.png | 1440 x 900 dark score-sorted capture. |
| README.md | Hosted-first English hierarchy plus score-sorted screenshot alternative text. |
| README.ko.md | Hosted-first Korean hierarchy plus score-sorted screenshot alternative text. |
| docs/superpowers/README.md | Index this plan while active, then retain only the human launch checklist after implementation. |
| docs/README.md | Expose the active plan, then link the archived verification record. |

---

### Task 1: Close the Production Durability and Bootstrap Gates

**Files:**
- Modify: deploy/production/compose.yaml:8-46
- Modify: deploy/production/.env.example:1-15
- Modify: deploy/production/README.md:1-42
- Modify: deploy/production/HUMAN_DEPLOY_GUIDE.md:1-205
- Reference: internal/ai/keys.go:12-86
- Reference: cmd/jobcron-user/main.go:32-89
- Reference: cmd/jobcron-import/main.go:36-177
- Reference: deploy/production/Dockerfile:5-19

**Interfaces:**
- Consumes: internal/ai.DefaultKeysPath() resolving ai_keys.json below /root/.config/jobcron in the current root-run Alpine image.
- Produces: a Compose volume named jobcron_config mounted at /root/.config/jobcron and an operator sequence using TUNNELED_DATABASE_URL plus OWNER_EMAIL.

- [ ] **Step 1: Run the static Compose contract and verify the current file fails**

Run from the repository root:

~~~sh
env \
  JOBCRON_IMAGE=example/jobcron:sha-test \
  DATABASE_URL=compose-validation-placeholder \
  SESSION_SECRET=dummy-session-secret \
  docker compose -f deploy/production/compose.yaml config --format json \
  > /tmp/jobcron-production-compose.json

node <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const cfg = JSON.parse(fs.readFileSync('/tmp/jobcron-production-compose.json', 'utf8'));
const mounts = cfg.services.app.volumes || [];
const keyMount = mounts.find((mount) => mount.target === '/root/.config/jobcron');
assert.ok(keyMount, 'app config mount missing');
assert.equal(keyMount.type, 'volume');
assert.equal(keyMount.source, 'jobcron_config');
assert.equal(cfg.volumes.jobcron_config.name, 'jobcron_config');
NODE
~~~

Expected: FAIL with AssertionError: app config mount missing.

- [ ] **Step 2: Add the stable directory-level named volume**

Add the app mount and preserve the two existing Caddy volumes exactly:

~~~yaml
services:
  app:
    image: "${JOBCRON_IMAGE:?set JOBCRON_IMAGE in .env}"
    pull_policy: always
    command:
      - --no-open
      - --host
      - 0.0.0.0
      - --port
      - "7777"
    environment:
      JOBCRON_ENV: production
      JOBCRON_HOST: 0.0.0.0
      JOBCRON_PORT: "7777"
      JOBCRON_NO_OPEN: "1"
      DATABASE_URL: "${DATABASE_URL:?set DATABASE_URL in .env}"
      SESSION_SECRET: "${SESSION_SECRET:?set SESSION_SECRET in .env}"
      JOBCRON_SCHEDULER_ENABLED: "1"
      JOBCRON_DAILY_SCRAPE_TIME: "05:00"
    expose:
      - "7777"
    volumes:
      - jobcron_config:/root/.config/jobcron
    restart: unless-stopped

  caddy:
    image: caddy:2.8-alpine
    depends_on:
      - app
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy_data:/data
      - caddy_config:/config
    restart: unless-stopped

volumes:
  jobcron_config:
    name: jobcron_config
  caddy_data:
  caddy_config:
~~~

The mount is the directory, not ai_keys.json itself, so SaveKeys can keep using atomic rename inside one filesystem.

- [ ] **Step 3: Re-run a complete static Compose assertion**

~~~sh
env \
  JOBCRON_IMAGE=example/jobcron:sha-test \
  DATABASE_URL=compose-validation-placeholder \
  SESSION_SECRET=dummy-session-secret \
  docker compose -f deploy/production/compose.yaml config --format json \
  > /tmp/jobcron-production-compose.json

node <<'NODE'
const assert = require('node:assert/strict');
const fs = require('node:fs');
const cfg = JSON.parse(fs.readFileSync('/tmp/jobcron-production-compose.json', 'utf8'));
const app = cfg.services.app;
const keyMount = app.volumes.find((mount) => mount.target === '/root/.config/jobcron');
assert.deepEqual(
  { type: keyMount.type, source: keyMount.source, target: keyMount.target },
  { type: 'volume', source: 'jobcron_config', target: '/root/.config/jobcron' }
);
assert.equal(cfg.volumes.jobcron_config.name, 'jobcron_config');
assert.ok(app.volumes.every((mount) => mount.type === 'volume'), 'app bind mount forbidden');

const caddyMounts = new Map(cfg.services.caddy.volumes.map((mount) => [mount.target, mount]));
assert.equal(caddyMounts.get('/data').source, 'caddy_data');
assert.equal(caddyMounts.get('/config').source, 'caddy_config');
assert.equal(caddyMounts.get('/etc/caddy/Caddyfile').type, 'bind');

for (const key of [
  'JOBCRON_DEMO',
  'JOBCRON_ADMIN_TOKEN',
  'JOBCRON_PROXY_SECRET',
  'JOBCRON_WORKNET_KEY',
]) {
  assert.ok(!(key in app.environment), key + ' must stay unset');
}
console.log('production Compose contract: PASS');
NODE
~~~

Expected: production Compose contract: PASS.

- [ ] **Step 4: Replace the stale image value in the public environment example**

Use this exact public-safe shape:

~~~dotenv
# Immutable image built and pushed from the approved release commit. The EC2
# host pulls this image; it does not build locally.
JOBCRON_IMAGE=<dockerhub-user>/jobcron:sha-<12-character-commit>

# AWS RDS PostgreSQL 18 connection string. Enter the real password only on EC2.
DATABASE_URL=postgres://jobcron_admin:<database-password>@<rds-endpoint>:5432/jobcron?sslmode=require

# Random application secret. Generate on EC2 with: openssl rand -base64 48
SESSION_SECRET=

# Leave these unset for the first production pass:
# JOBCRON_WORKNET_KEY
# JOBCRON_PROXY_SECRET
# JOBCRON_DEMO
# JOBCRON_ADMIN_TOKEN
~~~

- [ ] **Step 5: Update the production README with the durable-config contract**

Add these facts to Production assumptions and Files:

~~~markdown
- The app stores BYOK credentials below /root/.config/jobcron in the explicitly
  named jobcron_config Docker volume.
- Normal container recreation and docker compose down preserve the volume on the
  same EC2 host. docker compose down -v deletes it and is not a routine deploy
  command.
- Host loss is outside the volume's durability boundary; recover through a
  secure backup or re-enter the API key.

- compose.yaml mounts jobcron_config into the app and retains the existing Caddy
  volumes. It has no local build section and no local database service.
- HUMAN_DEPLOY_GUIDE.md keeps RDS private by using a localhost SSH tunnel for
  owner creation and optional import.
~~~

Change the dummy validation image to example/jobcron:sha-test and extend the expected rendered-config paragraph:

~~~markdown
The rendered config must mount jobcron_config at /root/.config/jobcron, retain
Caddy's caddy_data and caddy_config volumes, and include DATABASE_URL,
SESSION_SECRET, JOBCRON_ENV=production, JOBCRON_SCHEDULER_ENABLED=1, and
JOBCRON_DAILY_SCRAPE_TIME=05:00. It must not include demo mode, an admin token, a
Worknet key, or a proxy secret.
~~~

- [ ] **Step 6: Rewrite the human guide into an executable private-RDS sequence**

Replace the mutable image instructions with:

~~~~markdown
## 1. Build and publish the approved immutable image

From the approved release checkout on the operator's Mac:

~~~sh
git status --short
git rev-parse HEAD

RELEASE_TAG="sha-$(git rev-parse --short=12 HEAD)"
IMAGE="<dockerhub-user>/jobcron:$RELEASE_TAG"

docker login
docker buildx build \
  --platform linux/arm64 \
  -f deploy/production/Dockerfile \
  -t "$IMAGE" \
  --push .
docker buildx imagetools inspect "$IMAGE"
~~~

Stop if git status is not clean or the inspected image does not match the
approved release architecture and tag. Set JOBCRON_IMAGE in the EC2 .env file to
this exact immutable image.
~~~~

In the startup section, state that migrations happen before any operator command:

~~~markdown
The app must start successfully once before owner creation or import because app
startup applies PostgreSQL migrations.

After startup, verify docker volume inspect jobcron_config succeeds. Routine
deploys may use docker compose down, but must not use docker compose down -v.
~~~

Replace the current owner section with these exact sections:

~~~~markdown
## 8. Open a localhost-only tunnel to private RDS

Keep RDS public access disabled. In a dedicated terminal on the operator's Mac:

~~~sh
ssh -o ExitOnForwardFailure=yes -N \
  -L 127.0.0.1:15432:<rds-endpoint>:5432 \
  ec2-user@<ec2-public-host>
~~~

Leave that terminal running only for owner creation and optional import.

In a trusted local source checkout, export private values in the current shell:

~~~sh
export TUNNELED_DATABASE_URL='<postgresql-url-using-127.0.0.1:15432-and-sslmode-require>'
export OWNER_EMAIL='<owner-email>'
~~~

Do not paste the real URL or email into Git, issues, chat, or shared logs.

## 9. Create the owner before any import

Let jobcron-user prompt for the password so it does not enter shell history:

~~~sh
go run ./cmd/jobcron-user create-owner \
  --database-url "$TUNNELED_DATABASE_URL" \
  --email "$OWNER_EMAIL"
~~~

## 10. Optionally import the recovered SQLite state

Skip this section unless the human approved import and a current RDS snapshot
exists. Use the exact same OWNER_EMAIL as owner creation.

Run the source-count dry run first:

~~~sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL" \
  --dry-run
~~~

Review profile, postings, scores, bookmarks, not-interested state,
AI extractions, AI scores, and AI usage counts. If approved, run:

~~~sh
go run ./cmd/jobcron-import \
  --sqlite '<recovered-sqlite-path>' \
  --postgres "$TUNNELED_DATABASE_URL" \
  --owner-email "$OWNER_EMAIL"
~~~

The importer preserves the existing owner's password hash when the email
matches. It does not import ai_keys.json, sessions, owner passwords, or
production secrets.

Close the SSH tunnel and clear the private shell variables:

~~~sh
unset TUNNELED_DATABASE_URL OWNER_EMAIL
~~~

## 11. Enter the Anthropic key after volume durability exists

Sign in to jobcron.app and save the key through the application UI. The key
stays in /root/.config/jobcron/ai_keys.json inside jobcron_config. Re-enter it
after host loss unless a separate secure backup exists.
~~~~

Renumber Final checks after these sections. Do not add a direct public-RDS path,
JOBCRON_OWNER_PASSWORD example, or concrete owner identity.

- [ ] **Step 7: Run the existing operator tests**

Start the repository's PostgreSQL 18 development service and use its documented
host URL:

~~~sh
docker compose -f deploy/local/compose.yaml up -d
export TEST_DATABASE_URL='postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable'
JOBCRON_TEST_POSTGRES_URL="$TEST_DATABASE_URL" \
  go test ./cmd/jobcron-user ./cmd/jobcron-import -count=1
~~~

Expected: both packages pass, including secure prompt, owner creation,
password-hash preservation, dry-run counts, transaction rollback, and imported
state.

- [ ] **Step 8: Prove volume lifecycle on a disposable Docker engine**

Use a local test PostgreSQL URL in TEST_DATABASE_URL. The guard refuses to touch
an existing volume:

~~~sh
if docker volume inspect jobcron_config >/dev/null 2>&1; then
  echo "refusing: jobcron_config already exists in this Docker engine" >&2
  exit 2
fi

docker build \
  -f deploy/production/Dockerfile \
  -t local/jobcron:volume-test .

export JOBCRON_IMAGE=local/jobcron:volume-test
export CONTAINER_TEST_DATABASE_URL="$(printf 'postgres://postgres%s%s.%s.%s:55432/jobcron_dev?sslmode=disable' '@' host docker internal)"
export DATABASE_URL="$CONTAINER_TEST_DATABASE_URL"
export SESSION_SECRET=dummy-session-secret

docker compose -f deploy/production/compose.yaml up -d --pull never app
docker compose -f deploy/production/compose.yaml exec -T app \
  sh -c 'printf persisted > /root/.config/jobcron/persistence-sentinel'

docker compose -f deploy/production/compose.yaml up -d \
  --pull never --force-recreate app
docker compose -f deploy/production/compose.yaml exec -T app \
  test -f /root/.config/jobcron/persistence-sentinel

docker compose -f deploy/production/compose.yaml down
docker compose -f deploy/production/compose.yaml up -d --pull never app
docker compose -f deploy/production/compose.yaml exec -T app \
  test -f /root/.config/jobcron/persistence-sentinel

docker compose -f deploy/production/compose.yaml down
docker volume rm jobcron_config
docker compose -f deploy/local/compose.yaml down
unset JOBCRON_IMAGE DATABASE_URL SESSION_SECRET TEST_DATABASE_URL CONTAINER_TEST_DATABASE_URL
~~~

Expected: both file checks exit 0. Never substitute a real or fake API key for
the sentinel. If the initial guard fails, use another disposable Docker engine;
do not delete the pre-existing volume.

- [ ] **Step 9: Run the documentation publication gate and commit**

~~~sh
git add \
  deploy/production/compose.yaml \
  deploy/production/.env.example \
  deploy/production/README.md \
  deploy/production/HUMAN_DEPLOY_GUIDE.md
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner --no-color
git commit -m "fix: persist production config and document bootstrap"
~~~

Expected: the staged diff contains no deploy/demo or Dockerfile change, Gitleaks
reports no leaks, and the commit succeeds.

---

### Task 2: Establish the Signed-In Bookmarks Empty-State Contract

**Files:**
- Modify: web/bookmarks.html:71-121
- Modify: internal/server/bookmarks_test.go:66-108
- Test: internal/server/demo_test.go

**Interfaces:**
- Consumes: the existing empty-state copy and demo-only data-demo-empty target.
- Produces: [data-bookmarks-empty], present and hidden only beside a non-empty signed-in bookmark list.

- [ ] **Step 1: Add failing server/template tests**

Append these tests to internal/server/bookmarks_test.go:

~~~go
func TestBookmarksPageIncludesHiddenLiveEmptyState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	id := mustUpsert(t, st, listingPosting("saved-live-empty", "저장한 공고"))
	if err := st.SetBookmark(context.Background(), id, time.Now()); err != nil {
		t.Fatalf("SetBookmark: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookmarks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-bookmarks-empty hidden") {
		t.Error("non-empty signed-in bookmarks page lacks hidden live empty state")
	}
	if !strings.Contains(body, "여기서 다시 모아 볼 수 있어요.") {
		t.Error("signed-in live empty-state copy missing")
	}
}

func TestDemoBookmarksPageDoesNotRenderSignedInLiveEmptyState(t *testing.T) {
	srv, st := newTestServer(t, &fakeScraper{})
	mustUpsert(t, st, listingPosting("demo-candidate", "방문자 저장 후보"))
	srv.SetDemoMode(true)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bookmarks", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "data-demo-empty hidden") {
		t.Error("demo bookmarks page lost its existing demo empty-state target")
	}
	if strings.Contains(body, "data-bookmarks-empty") {
		t.Error("demo bookmarks page contains signed-in live empty-state markup")
	}
}
~~~

- [ ] **Step 2: Run the tests and verify the signed-in contract fails**

~~~sh
go test ./internal/server -run 'TestBookmarksPageIncludesHiddenLiveEmptyState|TestDemoBookmarksPageDoesNotRenderSignedInLiveEmptyState' -count=1
~~~

Expected: the signed-in test fails because data-bookmarks-empty is absent. The
demo assertion already documents the unchanged branch.

- [ ] **Step 3: Add only the signed-in hidden target**

After the existing ordered list and existing demo-only empty target, render:

~~~html
      {{if not demoMode}}
      <div class="empty" data-bookmarks-empty hidden>
        <p class="empty-title">아직 저장한 공고가 없어요.</p>
        <p class="empty-sub">마음에 드는 공고 옆의 북마크 아이콘을 눌러보세요.<br>여기서 다시 모아 볼 수 있어요.</p>
      </div>
      {{end}}
~~~

Do not edit the current data-demo-empty block or either branch's copy.

- [ ] **Step 4: Run focused template and demo regression tests**

~~~sh
go test ./internal/server -run 'TestBookmarksPage|TestDemo' -count=1
~~~

Expected: PASS, including hidden signed-in markup, existing initial empty state,
and demo routes.

- [ ] **Step 5: Commit the template contract**

~~~sh
git add web/bookmarks.html internal/server/bookmarks_test.go
git diff --cached --check
git commit -m "fix: add live bookmarks empty state"
~~~

Expected: one template/test commit with no JavaScript or demo deployment file.

---

### Task 3: Implement and Test the Signed-In Bookmark Lifecycle

**Files:**
- Create: web/testdata/bookmark-lifecycle.test.js
- Create: web/bookmark_test.go
- Modify: web/bookmark.js:25-107
- Modify: web/source-filter.js:36-169
- Reference: web/not-interested.js:70-83
- Reference: web/styles.css:765-770

**Interfaces:**
- Consumes: [data-bookmarks-empty], .postings, .count strong, .posting.removing, and the existing bookmark API response {bookmarked: boolean}.
- Produces: document event posting-list-change after a card is physically removed; source-filter.js listens to that event and recomputes from cards whose isConnected is true.

- [ ] **Step 1: Create the Go wrapper for the zero-package Node test**

Create web/bookmark_test.go:

~~~go
package web

import (
	"os/exec"
	"testing"
)

func TestBookmarkLifecycleBehavior(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("node is required for the zero-package bookmark lifecycle test")
	}
	cmd := exec.Command(node, "testdata/bookmark-lifecycle.test.js")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bookmark lifecycle harness: %v\n%s", err, out)
	}
}
~~~

- [ ] **Step 2: Create the lifecycle harness with ten explicit cases**

Create web/testdata/bookmark-lifecycle.test.js. Use Node's built-in assert,
fs, path, test, and vm modules. The harness must load the shipped bookmark.js
and source-filter.js bytes and provide:

~~~javascript
'use strict';

const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const test = require('node:test');
const vm = require('node:vm');

const bookmarkScript = fs.readFileSync(path.join(__dirname, '..', 'bookmark.js'), 'utf8');
const filterScript = fs.readFileSync(path.join(__dirname, '..', 'source-filter.js'), 'utf8');

class ClassList {
  constructor(names = []) { this.names = new Set(names); }
  add(name) { this.names.add(name); }
  contains(name) { return this.names.has(name); }
  toggle(name, force) {
    const on = force === undefined ? !this.names.has(name) : Boolean(force);
    if (on) this.names.add(name); else this.names.delete(name);
    return on;
  }
}

function makeHarness(options = {}) {
  const route = options.route || '/bookmarks';
  const demo = Boolean(options.demo);
  const sources = options.sources || ['jumpit'];
  const titles = options.titles || sources.map((_, index) => '공고 ' + (index + 1));
  const response = options.response || { ok: true, bookmarked: false };
  const documentListeners = new Map();
  const timers = [];
  const storage = new Map();
  if (demo) storage.set('jobcronDemoBookmarks', JSON.stringify(sources.map((_, index) => String(index + 1))));
  if (options.filterSource) storage.set('sourceFilter', options.filterSource);

  function listenersFor(target, type) {
    if (!target.listeners.has(type)) target.listeners.set(type, []);
    return target.listeners.get(type);
  }

  const count = { textContent: String(sources.length) };
  const list = { hidden: false };
  const pageEmpty = { hidden: true };
  const filterEmpty = { hidden: true, textContent: '' };
  const searchInput = {
    value: options.searchValue || '',
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
  };

  const pills = [
    { dataset: { source: '_all' }, textContent: '전체', classList: new ClassList(['source-pill', 'on']), attributes: {} },
    ...Array.from(new Set(sources)).map((source) => ({
      dataset: { source },
      textContent: source,
      classList: new ClassList(['source-pill']),
      attributes: {},
    })),
  ];
  for (const pill of pills) {
    pill.setAttribute = (name, value) => { pill.attributes[name] = value; };
  }

  const sourceContainer = {
    dataset: { emptyTemplate: '저장한 {label} 공고가 없어요.' },
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
    querySelectorAll(selector) { return selector === '.source-pill' ? pills : []; },
    querySelector(selector) {
      if (selector === '.source-pill.on') {
        return pills.find((pill) => pill.classList.contains('on')) || null;
      }
      const match = selector.match(/data-source="([^"]+)"/);
      return match ? pills.find((pill) => pill.dataset.source === match[1]) || null : null;
    },
  };

  let buttons;
  const cards = sources.map((source, index) => ({
    dataset: { source },
    hidden: false,
    isConnected: true,
    classList: new ClassList(['posting']),
    listeners: new Map(),
    addEventListener(type, fn) { listenersFor(this, type).push(fn); },
    emit(type) {
      for (const fn of this.listeners.get(type) || []) fn({ target: this });
    },
    remove() { this.isConnected = false; },
    closest(selector) { return selector === '.posting' ? this : null; },
    querySelector(selector) {
      if (selector === '.bookmark[data-posting-id]') return buttons[index];
      if (selector === '.posting-title') return { textContent: titles[index] };
      if (selector === '.posting-meta span') return { textContent: '회사 ' + (index + 1) };
      return null;
    },
  }));

  buttons = cards.map((card, index) => ({
    dataset: { postingId: String(index + 1) },
    disabled: false,
    classList: new ClassList(['bookmark', 'on']),
    attributes: { 'aria-pressed': 'true' },
    setAttribute(name, value) { this.attributes[name] = value; },
    closest(selector) {
      if (selector === '.bookmark') return this;
      if (selector === '.posting') return card;
      return null;
    },
  }));

  function liveCards() { return cards.filter((card) => card.isConnected); }
  function addDocumentListener(type, fn) {
    if (!documentListeners.has(type)) documentListeners.set(type, []);
    documentListeners.get(type).push(fn);
  }
  function emitDocument(type, event = {}) {
    for (const fn of documentListeners.get(type) || []) fn(event);
  }

  const document = {
    readyState: 'complete',
    body: { dataset: { demo: String(demo) } },
    addEventListener: addDocumentListener,
    dispatchEvent(event) { emitDocument(event.type, event); },
    emit: emitDocument,
    getElementById(id) {
      if (id === 'source-filter') return sourceContainer;
      if (id === 'source-filter-empty') return filterEmpty;
      if (id === 'posting-search') return searchInput;
      return null;
    },
    querySelector(selector) {
      if (selector === 'meta[name="csrf-token"]') return { getAttribute: () => 'csrf-test' };
      if (selector === '.count strong') return count;
      if (selector === '.empty') return pageEmpty;
      if (selector === '[data-bookmarks-empty]') return demo ? null : pageEmpty;
      if (selector === '.postings' || selector === 'ol.postings') return list;
      if (selector === '.excluded-box') return null;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === '.bookmark[data-posting-id]') return buttons;
      if (selector === '.posting[data-source]') return cards;
      if (selector === '.posting') return liveCards();
      if (selector === '.posting:not([hidden])') return liveCards().filter((card) => !card.hidden);
      if (selector === '.archive-day') return [];
      return [];
    },
  };

  const context = {
    CSS: { escape: (value) => value },
    CustomEvent: function CustomEvent(type) { this.type = type; },
    document,
    fetch: async () => {
      if (response.reject) throw new Error('network failure');
      return {
        ok: response.ok,
        status: response.status || (response.ok ? 200 : 500),
        json: async () => ({ bookmarked: response.bookmarked }),
      };
    },
    localStorage: {
      getItem: (key) => storage.has(key) ? storage.get(key) : null,
      setItem: (key, value) => storage.set(key, String(value)),
    },
    location: { pathname: route },
    setTimeout: (fn) => { timers.push(fn); return timers.length; },
    window: { CSS: { escape: (value) => value } },
  };
  vm.runInNewContext(bookmarkScript, context, { filename: 'bookmark.js' });
  vm.runInNewContext(filterScript, context, { filename: 'source-filter.js' });

  return { buttons, cards, count, document, filterEmpty, list, pageEmpty, storage, timers };
}

async function click(harness, index = 0) {
  harness.document.emit('click', { target: harness.buttons[index] });
  await new Promise((resolve) => setImmediate(resolve));
  await new Promise((resolve) => setImmediate(resolve));
}

test('successful signed-in DELETE removes after transition and reveals final empty state', async () => {
  const h = makeHarness();
  await click(h);
  assert.equal(h.cards[0].classList.contains('removing'), true);
  assert.equal(h.cards[0].isConnected, true);
  h.cards[0].emit('transitionend');
  assert.equal(h.cards[0].isConnected, false);
  assert.equal(h.count.textContent, '0');
  assert.equal(h.list.hidden, true);
  assert.equal(h.pageEmpty.hidden, false);
});

test('timeout removes when transitionend never fires', async () => {
  const h = makeHarness();
  await click(h);
  h.timers[0]();
  assert.equal(h.cards[0].isConnected, false);
});

test('HTTP failure restores the icon and leaves page state unchanged', async () => {
  const h = makeHarness({ response: { reject: true } });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
  assert.equal(h.buttons[0].classList.contains('on'), true);
  assert.equal(h.buttons[0].attributes['aria-pressed'], 'true');
  assert.equal(h.count.textContent, '1');
  assert.equal(h.pageEmpty.hidden, true);
});

test('contradictory final bookmarked true leaves the card', async () => {
  const h = makeHarness({ response: { ok: true, bookmarked: true } });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
  assert.equal(h.cards[0].classList.contains('removing'), false);
});

test('non-bookmarks route never removes a card', async () => {
  const h = makeHarness({ route: '/' });
  await click(h);
  assert.equal(h.cards[0].isConnected, true);
});

test('one of two removals updates count without showing page empty state', async () => {
  const h = makeHarness({ sources: ['jumpit', 'rallit'] });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.count.textContent, '1');
  assert.equal(h.list.hidden, false);
  assert.equal(h.pageEmpty.hidden, true);
});

test('source-filter empty message recomputes from connected cards', async () => {
  const h = makeHarness({ sources: ['jumpit', 'rallit'], filterSource: 'jumpit' });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.pageEmpty.hidden, true);
  assert.equal(h.filterEmpty.hidden, false);
  assert.equal(h.filterEmpty.textContent, '저장한 jumpit 공고가 없어요.');
});

test('text-search empty message recomputes from connected cards', async () => {
  const h = makeHarness({
    sources: ['jumpit', 'rallit'],
    titles: ['삭제할 공고', '남은 공고'],
    searchValue: '삭제할',
  });
  await click(h);
  h.cards[0].emit('transitionend');
  assert.equal(h.filterEmpty.hidden, false);
  assert.equal(h.filterEmpty.textContent, '검색 결과가 없어요.');
});

test('demo branch keeps immediate localStorage hiding and adds no transition', async () => {
  const h = makeHarness({ demo: true });
  await click(h);
  assert.equal(h.cards[0].hidden, true);
  assert.equal(h.cards[0].classList.contains('removing'), false);
  assert.equal(JSON.parse(h.storage.get('jobcronDemoBookmarks')).length, 0);
});

test('request completion re-enables the button', async () => {
  const h = makeHarness({ response: { ok: true, bookmarked: true } });
  await click(h);
  assert.equal(h.buttons[0].disabled, false);
});
~~~

- [ ] **Step 3: Run the lifecycle test and verify it fails against current JavaScript**

~~~sh
node web/testdata/bookmark-lifecycle.test.js
go test ./web -run TestBookmarkLifecycleBehavior -count=1
~~~

Expected: failures show that signed-in cards never receive removing, the count
and empty state do not change, and posting-list-change is absent.

- [ ] **Step 4: Add signed-in page synchronization to bookmark.js**

Insert these helpers after paintButton:

~~~javascript
  function signedInBookmarksPage() {
    return !demoMode() && location.pathname === '/bookmarks';
  }

  function syncSignedInBookmarks() {
    if (!signedInBookmarksPage()) return;
    var cards = Array.from(document.querySelectorAll('.posting'))
      .filter(function (card) { return card.isConnected; });
    var count = document.querySelector('.count strong');
    var list = document.querySelector('ol.postings');
    var empty = document.querySelector('[data-bookmarks-empty]');

    if (count) count.textContent = String(cards.length);
    if (list) list.hidden = cards.length === 0;
    if (empty) empty.hidden = cards.length !== 0;
    document.dispatchEvent(new CustomEvent('posting-list-change'));
  }

  function fadeRemove(el, afterRemove) {
    if (!el) return;
    el.classList.add('removing');
    var done = false;
    function go() {
      if (done) return;
      done = true;
      el.remove();
      if (afterRemove) afterRemove();
    }
    el.addEventListener('transitionend', go, { once: true });
    setTimeout(go, 260);
  }
~~~

Extend only the successful server-response branch:

~~~javascript
      .then(function (state) {
        var on = !!state.bookmarked;
        btn.classList.toggle('on', on);
        btn.setAttribute('aria-pressed', String(on));
        if (!on && signedInBookmarksPage()) {
          fadeRemove(btn.closest('.posting'), syncSignedInBookmarks);
        }
      })
~~~

Leave lines 67-72, the complete demo localStorage branch, byte-for-byte
behaviorally unchanged.

- [ ] **Step 5: Make source-filter.js recompute from connected cards**

Listen for the generic event beside the existing demo-state-change listener:

~~~javascript
    document.addEventListener('demo-state-change', applyFilters);
    document.addEventListener('posting-list-change', applyFilters);
~~~

Replace applyFilters and the empty-message signature with:

~~~javascript
    function applyFilters() {
      var source = activeSource();
      var q = searchInput ? searchInput.value.trim().normalize('NFC').toLowerCase() : '';
      var liveCards = cards.filter(function (card) {
        return card.el.isConnected;
      });

      var anyVisible = false;
      liveCards.forEach(function (c) {
        var srcMatch = source === ALL_KEY ||
          (',' + c.el.dataset.source + ',').indexOf(',' + source + ',') !== -1;
        var qMatch = q === '' || c.text.indexOf(q) !== -1;
        var visible = !c.el.hidden && srcMatch && qMatch;
        c.el.classList.toggle('filter-hidden', !visible);
        if (visible) anyVisible = true;
      });

      document.querySelectorAll('.archive-day').forEach(function (day) {
        var visible = day.querySelectorAll('.posting:not(.filter-hidden)').length;
        day.classList.toggle('filter-hidden', visible === 0);
      });

      markEmptyPills(container, liveCards);
      updateExcludedBox(q);
      updateEmptyMessage(source, q, anyVisible, liveCards.length > 0);
    }

    function updateEmptyMessage(source, q, anyVisible, pageHasPostings) {
      if (!emptyMsg) return;
      if (anyVisible || !pageHasPostings) {
        emptyMsg.hidden = true;
        emptyMsg.textContent = '';
        return;
      }
      if (q !== '') {
        emptyMsg.textContent = '검색 결과가 없어요.';
        emptyMsg.hidden = false;
      } else if (source !== ALL_KEY) {
        emptyMsg.textContent = emptyTemplate.replace('{label}', labelFor(container, source));
        emptyMsg.hidden = false;
      } else {
        emptyMsg.hidden = true;
        emptyMsg.textContent = '';
      }
    }
~~~

- [ ] **Step 6: Run direct and Go-wrapped JavaScript tests**

~~~sh
node web/testdata/bookmark-lifecycle.test.js
go test ./web -run 'TestBookmarkLifecycleBehavior|TestAIRerateLifecycleBehavior' -count=1
~~~

Expected: ten bookmark lifecycle cases and the existing AI rerate lifecycle pass.

- [ ] **Step 7: Run server and demo regression tests**

~~~sh
go test ./internal/server -run 'Bookmark|Bookmarks|Demo|SourceFilter' -count=1
go test ./web ./internal/server
~~~

Expected: PASS. No bookmark API, CSRF, per-user state, demo, 관심 없음, or source
filter regression.

- [ ] **Step 8: Commit the JavaScript lifecycle**

~~~sh
git add \
  web/bookmark.js \
  web/source-filter.js \
  web/testdata/bookmark-lifecycle.test.js \
  web/bookmark_test.go
git diff --cached --check
git commit -m "fix: synchronize signed-in bookmark removal"
~~~

Expected: no template, stylesheet, not-interested.js, or demo deployment change.

---

### Task 4: Publish Hosted-First READMEs and Score-Sorted Images

**Files:**
- Modify: docs/assets/screenshots/dashboard.png
- Modify: docs/assets/screenshots/dashboard-dark.png
- Modify: README.md:5-184
- Modify: README.ko.md:5-184
- Reference: scripts/preview-interactive.sh
- Reference: web/archive.html:55-57

**Interfaces:**
- Consumes: the verified local release candidate, the hosted-first product decision, /?sort=score, theme toggle, 1440 x 900 browser viewport, public-safe posting data.
- Produces: hosted-first English and Korean README journeys, two 1440 x 900 PNGs, and localized alternative text naming score order.

- [ ] **Step 1: Start an isolated signed-in local preview without opening a browser**

~~~sh
JOBCRON_PREVIEW_KEEP=1 scripts/preview-interactive.sh 17780
~~~

Expected startup output includes a temporary Preview state directory and
http://127.0.0.1:17780. Use only public-safe job data and a non-identifying
profile. Do not display or capture the AI-key page, owner identity, database
configuration, terminal, or browser storage.

- [ ] **Step 2: Load public-safe postings and verify score order**

Using /browse, walk the normal local UI to save the non-identifying profile and
perform one scrape if the preview database is empty. Navigate to:

~~~sh
PREVIEW_ORIGIN=http://127.0.0.1:17780
printf '%s\n' "$PREVIEW_ORIGIN/?sort=score"
~~~

At a 1440 x 900 viewport, verify:

- 점수순 is the active sort and has aria-current=true.
- 전체 is the active source pill.
- The text search is empty.
- The low-score section remains collapsed.
- Visible kept scores are monotonically non-increasing.
- No credential, owner email, profile detail, private endpoint, or production
  identifier is visible.
- Browser console and non-static network requests contain no error.

If the order assertion fails, stop and fix the release candidate. Do not stage a
misrepresentative screenshot.

- [ ] **Step 3: Capture light and dark assets through the real UI**

With /browse:

1. Click the theme control until light mode is visibly active.
2. Reload /?sort=score and recheck the active sort and descending scores.
3. Save the viewport screenshot as docs/assets/screenshots/dashboard.png.
4. Click the theme control until dark mode is visibly active.
5. Reload /?sort=score and repeat the checks.
6. Save the viewport screenshot as docs/assets/screenshots/dashboard-dark.png.

Do not crop or resize after capture; the browser viewport is the source of the
required dimensions.

- [ ] **Step 4: Verify both binary assets**

~~~sh
file \
  docs/assets/screenshots/dashboard.png \
  docs/assets/screenshots/dashboard-dark.png
sips -g pixelWidth -g pixelHeight \
  docs/assets/screenshots/dashboard.png \
  docs/assets/screenshots/dashboard-dark.png
~~~

Expected: both are non-interlaced PNG images with pixelWidth 1440 and
pixelHeight 900. Inspect both with the image viewer and confirm there is no
secret, personal data, clipped content, exposed browser chrome, or wrong sort.

- [ ] **Step 5: Reframe both READMEs and localize the alternative text**

Apply the same information hierarchy in both languages:

1. The opening describes Jobcron as a daily job briefing whose primary full app
   will be `jobcron.app`, rather than defining it as a local binary.
2. The deployment notice says `demo.jobcron.app` is live and read-only and that
   the full `jobcron.app` is coming soon. Remove the instruction to use localhost
   as the default until launch.
3. Keep product usage documentation in the main flow.
4. Group release-binary installation, first-run filesystem notes, the isolated
   writable preview, build-from-source instructions, and local PostgreSQL
   development under `## Advanced local use` in English and
   `## 고급 로컬 사용` in Korean.
5. Open each advanced section with these localized expectations:

~~~markdown
Most users should use `jobcron.app` once it launches. Local binaries and source
builds remain available for contributors and self-hosters who want to run the
writable app themselves.
~~~

~~~markdown
대부분의 사용자는 정식 출시 후 `jobcron.app`을 이용하면 됩니다. 로컬 바이너리와
소스 빌드는 쓰기 가능한 앱을 직접 실행하려는 기여자와 셀프 호스팅 사용자를 위해
계속 제공합니다.
~~~

Do not remove or alter a platform download command, local data-path note,
preview command, build command, SQLite behavior description, or local PostgreSQL
development command. Database convergence belongs to the linked post-launch
decision, not this task.

Then use this English picture block:

~~~html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/dashboard-dark.png">
  <img src="docs/assets/screenshots/dashboard.png" alt="Score-sorted all-postings page with source filters and AI evaluation chips">
</picture>
~~~

Use this Korean picture block:

~~~html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/screenshots/dashboard-dark.png">
  <img src="docs/assets/screenshots/dashboard.png" alt="점수순으로 정렬된 전체 공고 페이지의 소스 필터와 AI 평가 칩">
</picture>
~~~

- [ ] **Step 6: Render and inspect both READMEs**

Render README.md and README.ko.md using the repository's normal Markdown preview
without opening the user's default browser. Verify the light/dark picture source
switch, image proportions, alternative text, adjacent links, and surrounding
copy. Confirm the hosted path appears before advanced local operation, both
languages make the same availability claims, and no text claims `jobcron.app` is
already public.

- [ ] **Step 7: Run the publication gate and commit the asset refresh**

~~~sh
git add \
  README.md \
  README.ko.md \
  docs/assets/screenshots/dashboard.png \
  docs/assets/screenshots/dashboard-dark.png
git diff --cached --check
git diff --cached -- README.md README.ko.md
gitleaks git --staged --redact --no-banner --no-color
git commit -m "docs: make readmes hosted-first"
~~~

Expected: only the two README files and two intended PNG assets are committed.

---

### Task 5: Run Integrated QA and Hand Off the Human Launch Checklist

**Files:**
- Create: docs/superpowers/archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-verification.md
- Move: docs/superpowers/specs/260713-alpha-pre-launch-fixes.md
- Move: docs/superpowers/plans/260713-alpha-pre-launch-fixes-implementation-plan.md
- Modify: docs/superpowers/specs/260713-alpha-launch-human-blocked-steps.md
- Modify: docs/superpowers/README.md
- Modify: docs/README.md

**Interfaces:**
- Consumes: Tasks 1-4 and all 26 specification acceptance criteria.
- Produces: a verified local release commit plus one active human-only checklist; it does not execute any external launch action.

- [ ] **Step 1: Run the complete repository gate**

~~~sh
test -z "$(gofmt -l .)"
go vet ./...
docker compose -f deploy/local/compose.yaml up -d
export TEST_DATABASE_URL='postgres://postgres@localhost:55432/jobcron_dev?sslmode=disable'
JOBCRON_TEST_POSTGRES_URL="$TEST_DATABASE_URL" \
  go test ./cmd/jobcron-user ./cmd/jobcron-import -count=1
go test ./...
go build ./cmd/jobcron ./cmd/jobcron-user ./cmd/jobcron-import
node web/testdata/ai-rerate-lifecycle.test.js
node web/testdata/bookmark-lifecycle.test.js

env \
  JOBCRON_IMAGE=example/jobcron:sha-test \
  DATABASE_URL=compose-validation-placeholder \
  SESSION_SECRET=dummy-session-secret \
  docker compose -f deploy/production/compose.yaml config --quiet
~~~

Expected: every command exits 0.

- [ ] **Step 2: Walk the signed-in bookmark user paths with /browse**

Against the isolated local preview:

1. Seed at least two bookmarks from different sources.
2. On /bookmarks, remove one card and verify the exit motion, count 2 to 1, no
   navigation, and persistence after reload.
3. Activate a source pill that matches the remaining card, remove it, and verify
   the filter-specific empty message while another nonmatching bookmark exists.
4. Repeat with text search active.
5. Remove the final bookmark and verify count 0, hidden list, signed-in page-level
   empty state, and persistence after reload.
6. Outside /bookmarks, toggle a bookmark and verify bookmark.js does not remove
   the card.

Automated Node coverage supplies the HTTP failure, contradictory final state,
and timeout cases that /browse does not network-mock.

- [ ] **Step 3: Run Tier C frontend regression**

Use frontend-qa plus /browse. Walk /, /briefing, /bookmarks, /hidden, and
/profile, including its AI-provider and API-key controls:

- Desktop 1440 x 900 and mobile 390 x 844.
- Light and dark themes.
- No console errors, failed local assets, horizontal overflow, clipped cards, or
  stale counts.
- Click primary navigation, source pills, search, theme control, a posting link,
  bookmark control, and 관심 없음 control; verify the real destination or state.
- Inspect the adjacent 관심 없음 removal path because it shares the transition
  style.

Keep the preview server running for human inspection and report its URL. Do not
open the user's default browser.

- [ ] **Step 4: Verify the deployed demo read-only without changing it**

With /browse, open demo.jobcron.app and verify its bookmark button still writes
jobcronDemoBookmarks, hides the demo candidate immediately on /bookmarks, and
does not use the signed-in removing transition. Do not log in, deploy, edit
deploy/demo/, or mutate server state.

- [ ] **Step 5: Review the cumulative implementation diff**

~~~sh
git diff 4af4f14..HEAD -- \
  deploy/production \
  web \
  internal/server \
  README.md \
  README.ko.md \
  docs/assets/screenshots
git diff 4af4f14..HEAD --name-status
~~~

Confirm:

- No deploy/demo file changed.
- Dockerfile, internal/ai/keys.go, jobcron-user, jobcron-import, bookmark API,
  CSRF behavior, and .posting.removing CSS remain unchanged.
- Production Compose has one app config mount and unchanged Caddy mounts.
- Screenshot assets and alternative text match score order.
- Both READMEs present the hosted app before advanced local operation, preserve
  every local command, and make equivalent availability claims in both languages.
- The only frontend behavior change is signed-in /bookmarks lifecycle
  completion plus filter recomputation.

- [ ] **Step 6: Create a concise public-safe verification record**

Write the verification record with these sections and no raw logs:

~~~markdown
# Alpha Pre-Launch Fixes Verification

**Status:** Agent-owned implementation verified; human launch steps remain
**Verified commit:** <local-release-commit-sha>

## Delivered

- Durable same-host BYOK key volume
- Private-RDS owner and optional import runbook
- Signed-in in-place bookmark removal and synchronized page state
- Hosted-first English and Korean README journeys with score-sorted images

## Automated Verification

List commands, exit status, package/test counts, Compose static assertions, and
the safe sentinel lifecycle result.

## Browser Verification

List pages, viewports, themes, user flows, console/network result, and the local
preview URL.

## Publication Safety

Record staged-diff review, Gitleaks, semantic redaction review, and screenshot
inspection outcomes without embedding sensitive output.

## Remaining Human Boundary

Link the active human-blocked launch checklist. State that AWS, DNS, Docker Hub,
production secrets, owner identity, import approval, API-key entry, go-live, and
rollback were not executed.
~~~

The commit SHA placeholder is filled with the actual local release commit at
execution time; it is not an unresolved product decision.

- [ ] **Step 7: Archive completed agent-owned documents and keep the human checklist active**

Use git mv:

~~~sh
mkdir -p docs/superpowers/archive/2026-07-13-alpha-pre-launch-fixes
git mv \
  docs/superpowers/specs/260713-alpha-pre-launch-fixes.md \
  docs/superpowers/archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes.md
git mv \
  docs/superpowers/plans/260713-alpha-pre-launch-fixes-implementation-plan.md \
  docs/superpowers/archive/2026-07-13-alpha-pre-launch-fixes/260713-alpha-pre-launch-fixes-implementation-plan.md
~~~

Update the human checklist status to Ready for human execution and change its
implementation-prerequisite link to:

~~~markdown
**Implementation prerequisite:** [Verified alpha pre-launch fixes](260713-alpha-pre-launch-fixes-verification.md)
~~~

Update docs/superpowers/README.md so Active Work contains only:

~~~markdown
- [First production launch human-blocked steps](../../specs/260716-first-production-launch-human-blocked-steps.md)
~~~

Add the archived spec, plan, and verification under Recently Archived. Update
docs/README.md to remove the active plan link and add the verification link
under Implementation Work.

- [ ] **Step 8: Run the final documentation and secret gate**

~~~sh
git add \
  docs/README.md \
  docs/superpowers/README.md \
  docs/superpowers/specs/260713-alpha-launch-human-blocked-steps.md \
  docs/superpowers/archive/2026-07-13-alpha-pre-launch-fixes
git diff --cached --check
git diff --cached
gitleaks git --staged --redact --no-banner --no-color
~~~

Also run the public-repo semantic redaction review and manually inspect both PNG
files. Expected: no credential, private endpoint, owner identity, recovery path,
or unnecessary production-specific identifier.

- [ ] **Step 9: Commit the verified handoff**

~~~sh
git commit -m "docs: verify alpha pre-launch fixes"
git status --short
~~~

Expected: commit succeeds and the worktree is clean. Do not push. Return the
local release commit, preview URL, verification summary, and the active
human-blocked checklist to the user.

---

## Self-Review Traceability

| Acceptance criteria | Plan coverage |
|---|---|
| 1-5 durable configuration | Task 1 Steps 1-5 and 8 |
| 6-12 private-RDS bootstrap/import | Task 1 Steps 4-7 |
| 13 transition removal | Task 3 Steps 2, 4, and 6 |
| 14 failure rollback | Task 3 Steps 2, 3, and 6 |
| 15 contradictory final state | Task 3 Steps 2, 3, and 6 |
| 16 count 2 to 1 | Task 3 Step 2 and Task 5 Step 2 |
| 17 final page empty state | Task 2 and Task 3 Steps 2, 4, and 6 |
| 18 source/text filter recomputation | Task 3 Steps 2, 5, and 6; Task 5 Step 2 |
| 19 non-bookmarks routes | Task 3 Step 2 and Task 5 Step 2 |
| 20 demo unchanged | Task 2, Task 3 Step 2, and Task 5 Step 4 |
| 21-23 screenshot dimensions/order/safety | Task 4 Steps 2-5 |
| 24 adjacent behavior unchanged | Task 3 Step 7 and Task 5 Steps 1-5 |
| 25 full testing plan | Task 5 Steps 1-9 |
| 26 hosted-first, local-advanced README hierarchy | Task 4 Steps 5-7 and Task 5 Steps 3, 5, and 8 |

## Execution Order

Tasks 1, 2, and the failing-test portion of Task 3 are independently reviewable.
Complete Task 2 before Task 3's production JavaScript because bookmark.js
consumes the signed-in empty-state marker. Complete Task 4 only after Tasks 1-3
are verified so the screenshots represent the release candidate. Task 5 is the
single integration and documentation-lifecycle gate.
