// Package rallit is the 랠릿 (rallit.com) implementation of the
// scraper.Scraper interface. It targets the site's public listing JSON API
// (/api/v1/position) with the same two-phase shape used by 점핏 and
// 워크넷: a cheap listing call followed by per-posting detail fetches.
//
// See API_NOTES.md in this directory for the reverse-engineered endpoint
// shape, the level enum, and the filter parameters.
package rallit
