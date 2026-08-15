package taxonomy

import (
	"fmt"
	"strings"
)

type Taxonomy struct {
	AllowedCategories []string
	KeyTerms          map[string][]string

	aliasToCanonical map[string]string
	allowedLower     map[string]string
}

func New(allowed []string, keyTerms map[string][]string) *Taxonomy {
	allowedLower := make(map[string]string, len(allowed))
	for _, cat := range allowed {
		allowedLower[strings.ToLower(cat)] = cat
	}

	aliasToCanonical := make(map[string]string)
	for canonical, aliases := range keyTerms {
		canonicalLower := strings.ToLower(canonical)
		aliasToCanonical[canonicalLower] = canonicalLower
		for _, alias := range aliases {
			aliasToCanonical[strings.ToLower(alias)] = canonicalLower
		}
	}

	return &Taxonomy{
		AllowedCategories: allowed,
		KeyTerms:          keyTerms,
		aliasToCanonical:  aliasToCanonical,
		allowedLower:      allowedLower,
	}
}

func (t *Taxonomy) NormalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return nil, fmt.Errorf("no tags provided")
	}

	seen := make(map[string]bool)
	var result []string
	var unknownTags []string

	for _, tag := range tags {
		lower := strings.ToLower(tag)

		if canonical, isAlias := t.aliasToCanonical[lower]; isAlias {
			if !seen[canonical] {
				seen[canonical] = true
				result = append(result, canonical)
			}
			continue
		}

		if canonical, ok := t.allowedLower[lower]; ok {
			if !seen[strings.ToLower(canonical)] {
				seen[strings.ToLower(canonical)] = true
				result = append(result, canonical)
			}
			continue
		}

		unknownTags = append(unknownTags, tag)
	}

	if len(unknownTags) > 0 {
		suggestions := t.suggestForUnknown(unknownTags)
		return nil, fmt.Errorf(
			"unknown tag(s): %s; valid categories are: %s%s",
			strings.Join(unknownTags, ", "),
			strings.Join(t.AllowedCategories, ", "),
			suggestions,
		)
	}

	return result, nil
}

func (t *Taxonomy) ExpandQuery(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var result []string

	for _, term := range terms {
		lower := strings.ToLower(term)

		canonical, isKnown := t.aliasToCanonical[lower]
		if !isKnown {
			if !seen[lower] {
				seen[lower] = true
				result = append(result, term)
			}
			continue
		}

		group := t.KeyTerms[t.aliasToCanonical[lower]]
		for _, syn := range group {
			synLower := strings.ToLower(syn)
			if !seen[synLower] {
				seen[synLower] = true
				result = append(result, syn)
			}
		}

		if !seen[canonical] {
			seen[canonical] = true
			result = append(result, canonical)
		}
	}

	return result
}

func (t *Taxonomy) IsAllowed(tag string) bool {
	lower := strings.ToLower(tag)
	_, ok := t.allowedLower[lower]
	return ok
}

func (t *Taxonomy) suggestForUnknown(unknownTags []string) string {
	var suggestions []string
	for _, tag := range unknownTags {
		lower := strings.ToLower(tag)
		var matches []string
		for _, cat := range t.AllowedCategories {
			catLower := strings.ToLower(cat)
			if levenshtein(lower, catLower) <= 2 || strings.HasPrefix(catLower, lower) || strings.HasPrefix(lower, catLower) {
				matches = append(matches, cat)
			}
		}
		if len(matches) > 0 {
			suggestions = append(suggestions, fmt.Sprintf("%q suggests: %s", tag, strings.Join(matches, ", ")))
		}
	}
	if len(suggestions) == 0 {
		return ""
	}
	return "; " + strings.Join(suggestions, "; ")
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr := make([]int, lb+1)
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev = curr
	}
	return prev[lb]
}
