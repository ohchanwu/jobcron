// Package demoday scrapes 신입-relevant postings from the 데모데이
// (demoday.co.kr) board. The site is a Next.js front end backed by a
// public Supabase project; this scraper hits Supabase's REST endpoint
// directly because that is the same anonymous access path the
// in-page client uses. See API_NOTES.md for the recon trail.
package demoday
