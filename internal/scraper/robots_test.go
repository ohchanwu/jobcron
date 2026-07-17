package scraper

import "testing"

func TestRobotsAllows(t *testing.T) {
	tests := []struct {
		name, robots, path string
		allowed            bool
	}{
		{"empty", "", "/api/positions", true},
		{"other agent", "User-agent: bot\nDisallow: /", "/api/positions", true},
		{"unrelated disallow", "User-agent: *\nDisallow: /admin", "/api/positions", true},
		{"wildcard disallow", "User-agent: *\nDisallow: /api", "/api/positions", false},
		{"longer allow", "User-agent: *\nDisallow: /\nAllow: /api", "/api/x", true},
		{"empty disallow", "User-agent: *\nDisallow:", "/api/x", true},
		{"inline comment", "User-agent: *\nDisallow: /api # nope", "/api/positions", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := RobotsAllows([]byte(tc.robots), tc.path); got != tc.allowed {
				t.Fatalf("RobotsAllows() = %v, want %v", got, tc.allowed)
			}
		})
	}
}
