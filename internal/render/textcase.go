package render

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Case conversion is part of adder's output, so these rules are covered by the compatibility promise.

// words splits s into words. Any rune that is not a letter or digit separates words, and within a run of
// letters and digits a new word starts at a lowercase-or-digit to uppercase change ("userID" is user|ID)
// and before the last capital of an acronym followed by lowercase ("HTTPServer" is HTTP|Server).
// Digits stay with the word they follow ("HTTP2Server" is HTTP2|Server).
func words(s string) []string {
	var out []string
	runes := []rune(s)
	start := -1
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			if start >= 0 {
				out = append(out, string(runes[start:i]))
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
			continue
		}
		prev := runes[i-1]
		lowerToUpper := unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev))
		acronymEnd := unicode.IsUpper(r) && unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
		if lowerToUpper || acronymEnd {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	if start >= 0 {
		out = append(out, string(runes[start:]))
	}
	return out
}

// goInitialisms are the initialisms Go style keeps in a single case, from golint's list.
var goInitialisms = map[string]bool{
	"ACL": true, "API": true, "ASCII": true, "CPU": true, "CSS": true, "DNS": true, "EOF": true,
	"GUID": true, "HTML": true, "HTTP": true, "HTTPS": true, "ID": true, "IP": true, "JSON": true,
	"LHS": true, "QPS": true, "RAM": true, "RHS": true, "RPC": true, "SLA": true, "SMTP": true,
	"SQL": true, "SSH": true, "TCP": true, "TLS": true, "TTL": true, "UDP": true, "UI": true,
	"UID": true, "UUID": true, "URI": true, "URL": true, "UTF8": true, "VM": true, "XML": true,
	"XMPP": true, "XSRF": true, "XSS": true,
}

func capitalize(word string) string {
	r, size := utf8.DecodeRuneInString(word)
	return string(unicode.ToUpper(r)) + strings.ToLower(word[size:])
}

// goWord writes a word the way Go style expects: words already in capitals stay as written, known
// initialisms are capitalized, and everything else is title-cased.
func goWord(word string) string {
	upper := strings.ToUpper(word)
	if goInitialisms[upper] || (word == upper && utf8.RuneCountInString(word) > 1) {
		return upper
	}
	return capitalize(word)
}

func joinWords(s, sep string, transform func(i int, word string) string) string {
	ws := words(s)
	for i, w := range ws {
		ws[i] = transform(i, w)
	}
	return strings.Join(ws, sep)
}

func lowerWord(_ int, w string) string { return strings.ToLower(w) }
func upperWord(_ int, w string) string { return strings.ToUpper(w) }

func toSnakeCase(s string) string    { return joinWords(s, "_", lowerWord) }
func toKebabCase(s string) string    { return joinWords(s, "-", lowerWord) }
func toConstantCase(s string) string { return joinWords(s, "_", upperWord) }

func toPascalCase(s string) string {
	return joinWords(s, "", func(_ int, w string) string { return capitalize(w) })
}

func toCamelCase(s string) string {
	return joinWords(s, "", func(i int, w string) string {
		if i == 0 {
			return strings.ToLower(w)
		}
		return capitalize(w)
	})
}

func toGoPascalCase(s string) string {
	return joinWords(s, "", func(_ int, w string) string { return goWord(w) })
}

func toGoCamelCase(s string) string {
	return joinWords(s, "", func(i int, w string) string {
		if i == 0 {
			return strings.ToLower(w)
		}
		return goWord(w)
	})
}
