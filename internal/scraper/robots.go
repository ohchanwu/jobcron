package scraper

import (
	"bufio"
	"bytes"
	"strings"
)

// RobotsAllows reports whether path is allowed for our scraper by the given
// robots.txt content. It honors the "User-agent: *" group with prefix-match
// Disallow/Allow rules and longest-match-wins.
func RobotsAllows(content []byte, path string) bool {
	var disallow, allow []string
	inStar := false
	sc := bufio.NewScanner(bytes.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			inStar = value == "*"
		case "disallow":
			if inStar && value != "" {
				disallow = append(disallow, value)
			}
		case "allow":
			if inStar && value != "" {
				allow = append(allow, value)
			}
		}
	}
	blocked := longestPrefix(disallow, path)
	if blocked == 0 {
		return true
	}
	return longestPrefix(allow, path) >= blocked
}

func longestPrefix(rules []string, path string) int {
	best := 0
	for _, r := range rules {
		if len(r) > best && strings.HasPrefix(path, r) {
			best = len(r)
		}
	}
	return best
}
