// Package worknet is the 워크넷 (work.go.kr) implementation of the
// scraper.Scraper interface. It targets the Korean Public Data Portal's
// recruitment OpenAPI — a polite, XML-shaped public-data endpoint — with
// the same two-phase shape used by the 점핏 scraper: a cheap listing
// followed by per-posting detail fetches.
//
// See API_NOTES.md in this directory for the reverse-engineered endpoint
// shape, the field semantics that informed the parser, and the KECO
// occupation codes we filter on.
package worknet
