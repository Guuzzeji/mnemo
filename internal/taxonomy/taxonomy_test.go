package taxonomy

import (
	"strings"
	"testing"
)

var testAllowed = []string{"backend", "frontend", "devops", "architecture"}
var testKeyTerms = map[string][]string{
	"authentication": {"auth", "jwt", "session", "login"},
	"database":       {"db", "sql", "sqlite", "schema"},
}

func newTestTaxonomy() *Taxonomy {
	return New(testAllowed, testKeyTerms)
}

func TestNormalizeTags_ValidTagsPassThrough(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"backend", "frontend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"backend", "frontend"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeTags_AliasExpansion(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"auth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "authentication" {
		t.Errorf("got %v, want [authentication]", got)
	}
}

func TestNormalizeTags_AliasExpansionDatabase(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"db"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "database" {
		t.Errorf("got %v, want [database]", got)
	}
}

func TestNormalizeTags_CaseInsensitive(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"Backend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "backend" {
		t.Errorf("got %v, want [backend]", got)
	}
}

func TestNormalizeTags_AliasCaseInsensitive(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"Auth"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "authentication" {
		t.Errorf("got %v, want [authentication]", got)
	}
}

func TestNormalizeTags_Dedupe(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"auth", "jwt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "authentication" {
		t.Errorf("got %v, want [authentication]", got)
	}
}

func TestNormalizeTags_DedupeSameCanonical(t *testing.T) {
	tx := newTestTaxonomy()
	got, err := tx.NormalizeTags([]string{"auth", "login"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "authentication" {
		t.Errorf("got %v, want [authentication]", got)
	}
}

func TestNormalizeTags_UnknownTag_Error(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{"unknown"})
	if err == nil {
		t.Fatal("expected error for unknown tag, got nil")
	}
}

func TestNormalizeTags_UnknownTag_ListsValidCategories(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{"unknown"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, cat := range testAllowed {
		if !strings.Contains(msg, cat) {
			t.Errorf("error should mention valid category %q: %s", cat, msg)
		}
	}
}

func TestNormalizeTags_UnknownTag_NearMissSuggestion(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{"backendd"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "backend") {
		t.Errorf("error should suggest 'backend' for 'backendd': %s", msg)
	}
}

func TestNormalizeTags_UnknownTag_PrefixMatch(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{"front"})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "frontend") {
		t.Errorf("error should suggest 'frontend' for 'front': %s", msg)
	}
}

func TestNormalizeTags_EmptyInput_Error(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{})
	if err == nil {
		t.Fatal("expected error for empty tags")
	}
	if !strings.Contains(err.Error(), "no tags") {
		t.Errorf("error should mention 'no tags': %s", err.Error())
	}
}

func TestNormalizeTags_MixedValidAndUnknown(t *testing.T) {
	tx := newTestTaxonomy()
	_, err := tx.NormalizeTags([]string{"backend", "nope"})
	if err == nil {
		t.Fatal("expected error when any tag is unknown")
	}
}

func TestExpandQuery_ExpandsAliases(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"auth"})
	if !containsStr(got, "authentication") {
		t.Errorf("expected 'authentication' in %v", got)
	}
}

func TestExpandQuery_IncludesOriginalAlias(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"auth"})
	if !containsStr(got, "auth") {
		t.Errorf("expected 'auth' (original term) in %v", got)
	}
}

func TestExpandQuery_IncludesAllSynonyms(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"auth"})
	for _, syn := range []string{"authentication", "jwt", "session", "login"} {
		if !containsStr(got, syn) {
			t.Errorf("expected '%s' in %v", syn, got)
		}
	}
}

func TestExpandQuery_UnknownTermPassThrough(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"foobar"})
	if !containsStr(got, "foobar") {
		t.Errorf("unknown term should pass through: got %v", got)
	}
	if len(got) != 1 {
		t.Errorf("unknown term should pass through alone: got %v", got)
	}
}

func TestExpandQuery_CaseInsensitive(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"Auth"})
	if !containsStr(got, "authentication") {
		t.Errorf("case-insensitive expansion failed: got %v", got)
	}
}

func TestExpandQuery_MultipleTerms(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{"auth", "db"})
	if !containsStr(got, "authentication") {
		t.Errorf("expected 'authentication' in %v", got)
	}
	if !containsStr(got, "database") {
		t.Errorf("expected 'database' in %v", got)
	}
}

func TestExpandQuery_EmptyInput(t *testing.T) {
	tx := newTestTaxonomy()
	got := tx.ExpandQuery([]string{})
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestIsAllowed_ExactMatch(t *testing.T) {
	tx := newTestTaxonomy()
	if !tx.IsAllowed("backend") {
		t.Error("expected 'backend' to be allowed")
	}
}

func TestIsAllowed_CaseInsensitive(t *testing.T) {
	tx := newTestTaxonomy()
	if !tx.IsAllowed("Backend") {
		t.Error("expected 'Backend' to be allowed")
	}
	if !tx.IsAllowed("FRONTEND") {
		t.Error("expected 'FRONTEND' to be allowed")
	}
}

func TestIsAllowed_NotAllowed(t *testing.T) {
	tx := newTestTaxonomy()
	if tx.IsAllowed("unknown") {
		t.Error("expected 'unknown' to not be allowed")
	}
}

func TestLevenshteinDistance_Same(t *testing.T) {
	if d := levenshtein("abc", "abc"); d != 0 {
		t.Errorf("expected 0, got %d", d)
	}
}

func TestLevenshteinDistance_OneEdit(t *testing.T) {
	if d := levenshtein("cat", "bat"); d != 1 {
		t.Errorf("expected 1, got %d", d)
	}
}

func TestLevenshteinDistance_Insert(t *testing.T) {
	if d := levenshtein("ab", "abc"); d != 1 {
		t.Errorf("expected 1, got %d", d)
	}
}

func TestLevenshteinDistance_CompletelyDifferent(t *testing.T) {
	if d := levenshtein("abc", "xyz"); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
}

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
