// Package naver is the Naver careers (recruit.navercorp.com) implementation
// of the scraper.Scraper interface. Naver's job listings live behind a
// single JSON endpoint (/rcrt/loadJobList.do) that returns the full
// open-posting universe, so this is a single-phase scraper — FetchDetail
// is a no-op.
//
// See API_NOTES.md in this directory for the reverse-engineered endpoint
// shape and the entry-type codes.
package naver
