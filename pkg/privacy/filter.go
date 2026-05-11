package privacy

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Filter struct {
	patterns []string
}

func NewFilterFromFile(path string) (*Filter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &Filter{patterns: patterns}, nil
}

func NewFilter(patterns []string) *Filter {
	return &Filter{patterns: patterns}
}

func (f *Filter) IsBlocked(content string) (bool, string) {
	for _, p := range f.patterns {
		if matchPattern(p, content) {
			return true, p
		}
	}
	return false, ""
}

func matchPattern(pattern, s string) bool {
	if !strings.ContainsAny(pattern, "*?[") {
		return strings.Contains(strings.ToLower(s), strings.ToLower(pattern))
	}
	matched, _ := filepath.Match(pattern, s)
	return matched
}

func (f *Filter) Patterns() []string {
	return f.patterns
}
