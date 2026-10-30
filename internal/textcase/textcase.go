// Package textcase converts between the ways words are joined in names, such as snake_case and PascalCase.
package textcase

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

// goInitialisms are the initialisms Go style keeps in a single case: staticcheck's ST1003 default list as of
// adder v1.0, frozen so that upgrading adder never renames a generated identifier. Words already in capitals stay
// in capitals whatever the list says.
var goInitialisms = map[string]bool{
	"ACL": true, "API": true, "ASCII": true, "CPU": true, "CSS": true, "DNS": true, "EOF": true, "GUID": true,
	"HTML": true, "HTTP": true, "HTTPS": true, "ID": true, "IP": true, "JSON": true, "QPS": true, "RAM": true,
	"RPC": true, "SLA": true, "SMTP": true, "SQL": true, "SSH": true, "TCP": true, "TLS": true, "TTL": true,
	"UDP": true, "UI": true, "GID": true, "UID": true, "UUID": true, "URI": true, "URL": true, "UTF8": true,
	"VM": true, "XML": true, "XMPP": true, "XSRF": true, "XSS": true, "SIP": true, "RTP": true, "AMQP": true,
	"DB": true, "TS": true,
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

// Snake joins the words of s in lowercase with underscores, as in user_id.
func Snake(s string) string { return joinWords(s, "_", lowerWord) }

// Kebab joins the words of s in lowercase with hyphens, as in user-id.
func Kebab(s string) string { return joinWords(s, "-", lowerWord) }

// Constant joins the words of s in uppercase with underscores, as in USER_ID.
func Constant(s string) string { return joinWords(s, "_", upperWord) }

// Pascal joins the words of s with each word capitalized, as in UserId.
func Pascal(s string) string {
	return joinWords(s, "", func(_ int, w string) string { return capitalize(w) })
}

// Camel joins the words of s with each word but the first capitalized, as in userId.
func Camel(s string) string {
	return joinWords(s, "", func(i int, w string) string {
		if i == 0 {
			return strings.ToLower(w)
		}
		return capitalize(w)
	})
}

// GoPascal joins the words of s in Go style with each word capitalized, as in UserID.
func GoPascal(s string) string {
	return joinWords(s, "", func(_ int, w string) string { return goWord(w) })
}

// GoCamel joins the words of s in Go style with each word but the first capitalized, as in userID.
func GoCamel(s string) string {
	return joinWords(s, "", func(i int, w string) string {
		if i == 0 {
			return strings.ToLower(w)
		}
		return goWord(w)
	})
}
