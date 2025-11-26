package profanity

import (
	"embed"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
)

var (
	// Global instance for reuse (thread-safe)
	defaultFilter *ProfanityFilter
	once          sync.Once
)

//go:embed words.json
var jsonData embed.FS

func LoadBannedWords() []string {
	data, err := jsonData.ReadFile("words.json")
	if err != nil {
		log.Fatalf("Failed to read embedded file: %s", err)
	}

	var bannedWords []string
	if err := json.Unmarshal(data, &bannedWords); err != nil {
		log.Fatalf("Failed to unmarshal JSON: %s", err)
	}
	return bannedWords
}

type ProfanityFilter struct {
	regex *regexp.Regexp
}

func NewProfanityFilter() *ProfanityFilter {
	once.Do(func() {
		defaultFilter = &ProfanityFilter{
			regex: buildMasterRegex(),
		}
	})

	return defaultFilter
}

func (pf *ProfanityFilter) ContainsProfanity(text string) bool {
	if text == "" {
		return false
	}

	// Fast path: normalize once
	normalized := normalizeText(text)
	return pf.regex.MatchString(normalized)
}

func (pf *ProfanityFilter) ContainsProfanityNormalized(normalized string) bool {
	return normalized != "" && pf.regex.MatchString(normalized)
}

func normalizeText(text string) string {
	// Common replacements to defeat obfuscation
	s := strings.ToLower(text)
	s = strings.Map(func(r rune) rune {
		switch r {
		case 'á', 'à', 'â', 'ä', 'ã', 'å':
			return 'a'
		case 'é', 'è', 'ê', 'ë':
			return 'e'
		case 'í', 'ì', 'î', 'ï':
			return 'i'
		case 'ó', 'ò', 'ô', 'ö', 'õ':
			return 'o'
		case 'ú', 'ù', 'û', 'ü':
			return 'u'
		case 'ñ':
			return 'n'
		case 'ç':
			return 'c'
		default:
			return r
		}
	}, s)

	// Replace common leetspeak in one pass
	s = strings.NewReplacer(
		"@", "a", "4", "a", "á", "a",
		"3", "e", "€", "e",
		"1", "i", "!", "i", "|", "i", "¡", "i",
		"0", "o", "()", "o", "[]", "o",
		"$", "s", "5", "s", "z", "s",
		"7", "t", "+", "t",
		"ph", "f",
		"ck", "k", "kk", "k",
		"\\b", "b", // common backslash abuse
	).Replace(s)

	// Collapse whitespace and common separators
	s = regexp.MustCompile(`[\s_.\-*/\\|]+`).ReplaceAllString(s, " ")

	return s
}

func buildMasterRegex() *regexp.Regexp {
	patterns := []string{}

	for _, base := range LoadBannedWords() {
		variants := generateVariants(base)
		for _, variant := range variants {
			// Escape special regex chars, then make flexible
			escaped := regexp.QuoteMeta(variant)
			// Allow optional separators inside words: f.u.c.k → f[ ._-]*u[ ._-]*c[ ._-]*k
			flexible := regexp.MustCompile(`(?i)(.)(.)`).ReplaceAllString(escaped, `${1}[^\\p{L}]*${2}`)
			patterns = append(patterns, flexible)
		}
	}

	// Join all with | and add word boundaries
	expression := `(?:^|\W)(` + strings.Join(patterns, "|") + `)(?:$|\W)`

	// Compile once with optimizations
	re := regexp.MustCompile(expression)
	return re
}

// generateVariants creates common obfuscations of a word
func generateVariants(word string) []string {
	if word == "" {
		return nil
	}

	word = strings.ToLower(word)

	variants := make(map[string]struct{})
	variants[word] = struct{}{}

	// Common substitutions
	subs := map[rune][]rune{
		'a': {'a', '@', '4'},
		'e': {'e', '3'},
		'i': {'i', '1', '!', '|'},
		'o': {'o', '0'},
		's': {'s', '$', '5', 'z'},
		't': {'t', '7', '+'},
		'g': {'g', '9'},
		'b': {'b', '8'},
	}

	var generate func(string, int)
	generate = func(current string, idx int) {
		if idx >= len(word) {
			variants[current] = struct{}{}
			return
		}
		r := rune(word[idx])
		if replacements, ok := subs[r]; ok {
			for _, repl := range replacements {
				generate(current+string(repl), idx+1)
			}
		} else {
			generate(current+string(r), idx+1)
		}
	}

	generate("", 0)

	// Add doubled letters: fuuck, shiit
	result := make([]string, 0, len(variants)*2)
	for v := range variants {
		result = append(result, v)
		if len(v) <= 10 {
			for i := 1; i < len(v); i++ {
				doubled := v[:i] + string(v[i-1]) + v[i:]
				result = append(result, doubled)
			}
		}
	}

	return result
}
