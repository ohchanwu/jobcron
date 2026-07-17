# Daangn Canonical Role URLs

## Problem

Daangn postings currently use `https://team.daangn.com/jobs/{id}/`. Daangn now redirects those
links to `https://careers.daangn.com/jobs/role/{id}/`. The destination is the correct posting,
but Jobcron emits a legacy URL and the live URL contract rejects the cross-host redirect.

## Decision

Jobcron will emit Daangn's canonical careers URL directly:

```text
https://careers.daangn.com/jobs/role/{source_posting_id}/
```

`LinkSite` is used only by Daangn, so its path changes from `/jobs/{id}/` to
`/jobs/role/{id}/`. Daangn's `SiteURL` changes to `https://careers.daangn.com`. No new strategy,
dependency, configuration, database migration, or redirect allow-list is introduced.

## Rejected Alternatives

- Keep the legacy URL and permit the redirect: leaves stale links in stored postings and weakens
  the live contract.
- Use the Greenhouse hosted-board URL: Daangn's curated site URL is the user-facing canonical
  destination and already preserves the exact role ID.

## Verification

- The Daangn fixture test locks the canonical host and `/jobs/role/{id}/` path.
- The live Greenhouse URL test must end on `careers.daangn.com`, return the posting page, and
  contain the exact posting ID.
- A real browser must open representative links and show the matching role ID and title.
- Full static, unit, race, coverage, live Greenhouse, and publication-security gates pass.
