package leakdetector

import "regexp"

type Detector struct {
	patterns []PatternInfo
	compiled []*regexp.Regexp
}

func New() *Detector {
	d := &Detector{}
	d.loadDefaults()
	d.compile()
	return d
}

func (d *Detector) loadDefaults() {
	d.patterns = []PatternInfo{
		{Name: "openai-api-key", Pattern: `\b(sk-[a-zA-Z0-9]{20,})\b`, Type: "api_key"},
		{Name: "generic-api-key", Pattern: `(?i)(api[_-]?key|apikey)\s*[:=]\s*['"]?[a-zA-Z0-9_-]{20,}['"]?`, Type: "api_key"},
		{Name: "password", Pattern: `(?i)(password|passwd|pwd)\s*[:=]\s*['"]?[^\s'"]+['"]?`, Type: "password"},
		{Name: "token-secret", Pattern: `(?i)(token|secret)\s*[:=]\s*['"]?[a-zA-Z0-9_-]{16,}['"]?`, Type: "token"},
		{Name: "private-key", Pattern: `(?i)-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`, Type: "private_key"},
	}
}

func (d *Detector) compile() {
	for _, p := range d.patterns {
		re, err := regexp.Compile(p.Pattern)
		if err == nil {
			d.compiled = append(d.compiled, re)
		}
	}
}

func (d *Detector) Scan(content string) []Match {
	var matches []Match
	for i, re := range d.compiled {
		if i >= len(d.patterns) {
			break
		}
		found := re.FindAllString(content, -1)
		for _, f := range found {
			matches = append(matches, Match{
				Pattern: f,
				Type:    d.patterns[i].Type,
			})
		}
	}
	return matches
}

func (d *Detector) ScanAndClean(content string) (string, []Match) {
	matches := d.Scan(content)
	cleaned := content
	for _, m := range matches {
		cleaned = regexp.MustCompile(regexp.QuoteMeta(m.Pattern)).ReplaceAllString(cleaned, "[REDACTED]")
	}
	return cleaned, matches
}
