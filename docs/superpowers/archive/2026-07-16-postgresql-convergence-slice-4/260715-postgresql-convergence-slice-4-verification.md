# PostgreSQL Convergence Slice 4 Verification

Verified on 2026-07-16 against implementation range
`6f2c4929d9a9ab5a064383772aaa4e3f2319e398..6a7d579e323191df9da028626c6454087c7fbb41`.

## Delivered Contract

- `jobcron-import` snapshots and fingerprints a locked legacy SQLite source.
- Dry-run is the default and reports collisions without writing.
- `--apply` copies all supported categories and the optional encrypted
  credential in one PostgreSQL transaction.
- The importer verifies committed values, records a source fingerprint, treats
  the same fingerprint as readback-only, and refuses a different fingerprint.
- Sequence repair is transactional and resumes above the largest imported
  posting ID.
- Normal `jobcron` startup is PostgreSQL-only. Legacy SQLite access remains
  explicit and limited to the importer and compatibility tests.
- Release archives contain both `jobcron` and `jobcron-import` for every
  supported target.

## Automated Evidence

The final polecat tree passed:

- `go vet ./...`;
- builds for all three shipped commands;
- the full PostgreSQL-backed suite with no importer skips;
- `go test -race ./... -count=1`;
- the standalone scripts and preview suite;
- Actionlint and `goreleaser check`;
- a real GoReleaser snapshot plus inspection of all five archives;
- staged and full-range Gitleaks scans; and
- Git diff and formatting hygiene checks.

Mayor independently reran the importer package verbosely against the disposable
PostgreSQL database and confirmed no skipped test. The real `jobcron --help`
binary exited successfully without creating home, config, data, database, or
Docker state.

## User-Path Evidence

The isolated preview started on loopback with its own PostgreSQL database and
credential key. A headless browser loaded the postings and profile pages with
no console errors, changed and saved the career and preferred-work fields, then
reopened the profile and observed both values from PostgreSQL. Preview shutdown
removed the process, listener, and temporary database.

## Carry-Forward Decisions

- Packaged release users run the included `jobcron-import`; a source checkout is
  not required.
- The managed local application must bootstrap PostgreSQL, the fixed local
  owner, and the protected local credential key before importing.
- Production deployment work remains in Slice 5. This slice performed no image
  publication or AWS, RDS, EC2, DNS, or production data mutation.
