// Package daangn scrapes 신입 IT postings from 당근 (Karrot Market)
// via its public Greenhouse board. Greenhouse exposes a JSON API
// with no authentication, includes JD content in the listing
// response with `?content=true`, and tags every posting with rich
// metadata — including a dedicated `Engineer: yes/no` flag and a
// `Prior Experience: 신입 / 경력 / 신입/경력` field that make
// filtering down to 신입 IT trivial. See API_NOTES.md for the recon
// trail.
package daangn
