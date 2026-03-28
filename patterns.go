package clitest

import (
	"fmt"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{([^}:]+)(?::([^}]+))?\}\}`)

// MatchOutput compares actual output to expected using matchType (e.g. ORDERED_LINES).
func MatchOutput(expectedContent, actualOutput string, matchType string, patterns map[string]string) error {
	switch matchType {
	case "EXACT":
		if actualOutput != expectedContent {
			return fmt.Errorf("output mismatch\nExpected:\n%s\nActual:\n%s", expectedContent, actualOutput)
		}
		return nil
	case "SUBSTRING":
		if !strings.Contains(actualOutput, expectedContent) {
			return fmt.Errorf("missing substring:\n%q\nActual:\n%s", expectedContent, actualOutput)
		}
		return nil
	case "REGEX":
		matched, err := regexp.MatchString(expectedContent, actualOutput)
		if err != nil {
			return fmt.Errorf("invalid regex: %v", err)
		}
		if !matched {
			return fmt.Errorf("regex not matched: %q\nActual:\n%s", expectedContent, actualOutput)
		}
		return nil
	case "ORDERED_LINES":
		expLines := strings.Split(strings.TrimSpace(expectedContent), "\n")
		actLines := strings.Split(strings.TrimSpace(actualOutput), "\n")
		currentPos := 0
		const maxLookahead = 15
		for i, exp := range expLines {
			exp = strings.TrimSpace(exp)
			if exp == "" {
				continue
			}
			found := false
			searchEnd := min(currentPos+maxLookahead, len(actLines))
			for pos := currentPos; pos < searchEnd; pos++ {
				actual := strings.TrimSpace(actLines[pos])
				if matchLine(exp, actual, patterns) {
					found = true
					currentPos = pos + 1
					break
				}
			}
			if !found {
				var context []string
				for j := max(0, currentPos-3); j < min(len(actLines), currentPos+10); j++ {
					context = append(context, fmt.Sprintf("  %d: %s", j+1, actLines[j]))
				}
				return fmt.Errorf("ORDERED_LINES broken at expected line %d: %q\nNext few actual lines:\n%s", i+1, exp, strings.Join(context, "\n"))
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported match type: %s", matchType)
	}
}

func matchLine(expected, actual string, patterns map[string]string) bool {
	if !placeholderRe.MatchString(expected) {
		return actual == expected
	}
	var b strings.Builder
	last := 0
	for _, loc := range placeholderRe.FindAllStringSubmatchIndex(expected, -1) {
		b.WriteString(flexLiteralRegex(expected[last:loc[0]]))
		name := expected[loc[2]:loc[3]]
		var fragment string
		if loc[4] >= 0 && loc[5] >= 0 {
			fragment = expected[loc[4]:loc[5]]
		} else if p, ok := patterns[name]; ok {
			fragment = p
		} else {
			fragment = regexp.QuoteMeta(expected[loc[0]:loc[1]])
		}
		b.WriteString(fragment)
		last = loc[1]
	}
	b.WriteString(flexLiteralRegex(expected[last:]))
	regexPattern := b.String()
	matched, err := regexp.MatchString(regexPattern, actual)
	if err != nil {
		return false
	}
	return matched
}

// flexLiteralRegex quotes literal text for regex matching but maps runs of spaces to \s+
// so tabular CLI output with variable padding still matches.
func flexLiteralRegex(s string) string {
	var b strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == ' ' || s[i] == '\t' {
			b.WriteString(`\s+`)
			for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
				i++
			}
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' {
			j++
		}
		b.WriteString(regexp.QuoteMeta(s[i:j]))
		i = j
	}
	return b.String()
}
