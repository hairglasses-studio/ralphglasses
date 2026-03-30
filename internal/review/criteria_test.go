package review

import (
	"regexp"
	"testing"
)

func TestCriterion_Match(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		input   string
		want    bool
	}{
		{"matches simple", `hello`, "hello world", true},
		{"no match", `hello`, "goodbye world", false},
		{"matches regex", `\d+`, "line 42", true},
		{"empty pattern matches all", ``, "anything", true},
		{"anchored pattern", `^func`, "func main()", true},
		{"anchored pattern no match", `^func`, "  func main()", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Criterion{Pattern: regexp.MustCompile(tt.pattern)}
			if got := c.Match(tt.input); got != tt.want {
				t.Errorf("Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewCriteriaSet_Empty(t *testing.T) {
	cs := NewCriteriaSet()
	if cs.Len() != 0 {
		t.Errorf("Len() = %d, want 0", cs.Len())
	}
	if all := cs.All(); len(all) != 0 {
		t.Errorf("All() = %d items, want 0", len(all))
	}
}

func TestCriteriaSet_Add(t *testing.T) {
	cs := NewCriteriaSet()
	c1 := &Criterion{ID: "A"}
	c2 := &Criterion{ID: "B"}

	cs.Add(c1)
	if cs.Len() != 1 {
		t.Errorf("Len() = %d, want 1", cs.Len())
	}

	cs.Add(c2)
	if cs.Len() != 2 {
		t.Errorf("Len() = %d, want 2", cs.Len())
	}
}

func TestCriteriaSet_AddMultiple(t *testing.T) {
	cs := NewCriteriaSet()
	cs.Add(&Criterion{ID: "A"}, &Criterion{ID: "B"}, &Criterion{ID: "C"})
	if cs.Len() != 3 {
		t.Errorf("Len() = %d, want 3", cs.Len())
	}
}

func TestCriteriaSet_Merge(t *testing.T) {
	a := NewCriteriaSet()
	a.Add(&Criterion{ID: "A1"}, &Criterion{ID: "A2"})

	b := NewCriteriaSet()
	b.Add(&Criterion{ID: "B1"})

	a.Merge(b)
	if a.Len() != 3 {
		t.Errorf("merged Len() = %d, want 3", a.Len())
	}
}

func TestCriteriaSet_MergeEmpty(t *testing.T) {
	a := NewCriteriaSet()
	a.Add(&Criterion{ID: "A1"})

	b := NewCriteriaSet()
	a.Merge(b)
	if a.Len() != 1 {
		t.Errorf("merged Len() = %d, want 1", a.Len())
	}
}

func TestCriteriaSet_All_ReturnsCopy(t *testing.T) {
	cs := NewCriteriaSet()
	cs.Add(&Criterion{ID: "A"})

	all := cs.All()
	all[0] = &Criterion{ID: "MUTATED"}

	// Original should be unchanged.
	if cs.All()[0].ID != "A" {
		t.Error("All() should return a copy, not a reference to internal slice")
	}
}

func TestCriteriaSet_ByCategory_Filtering(t *testing.T) {
	cs := NewCriteriaSet()
	cs.Add(
		&Criterion{ID: "S1", Category: CategorySecurity},
		&Criterion{ID: "S2", Category: CategorySecurity},
		&Criterion{ID: "C1", Category: CategoryCorrectness},
		&Criterion{ID: "T1", Category: CategoryStyle},
	)

	tests := []struct {
		cat  Category
		want int
	}{
		{CategorySecurity, 2},
		{CategoryCorrectness, 1},
		{CategoryStyle, 1},
		{Category("nonexistent"), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.cat), func(t *testing.T) {
			got := cs.ByCategory(tt.cat)
			if len(got) != tt.want {
				t.Errorf("ByCategory(%s) = %d, want %d", tt.cat, len(got), tt.want)
			}
			for _, c := range got {
				if c.Category != tt.cat {
					t.Errorf("criterion %s has category %s, want %s", c.ID, c.Category, tt.cat)
				}
			}
		})
	}
}

func TestSecurityCriteria_Count(t *testing.T) {
	cs := SecurityCriteria()
	if cs.Len() != 4 {
		t.Errorf("SecurityCriteria Len() = %d, want 4", cs.Len())
	}
	for _, c := range cs.All() {
		if c.Category != CategorySecurity {
			t.Errorf("criterion %s has category %s, want security", c.ID, c.Category)
		}
	}
}

func TestSecurityCriteria_SEC001(t *testing.T) {
	cs := SecurityCriteria()
	sec001 := findCriterion(t, cs, "SEC001")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"password match", `password = "my_secret_value_here"`, true},
		{"api_key match", `api_key = "abcdefgh12345678"`, true},
		{"token match", `token = "some_token_value_here"`, true},
		{"env var no match", `password = os.Getenv("DB_PASS")`, false},
		{"short value no match", `password = "short"`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sec001.Match(tt.input); got != tt.want {
				t.Errorf("SEC001.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSecurityCriteria_SEC004(t *testing.T) {
	cs := SecurityCriteria()
	sec004 := findCriterion(t, cs, "SEC004")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"RSA private key", "-----BEGIN RSA PRIVATE KEY-----", true},
		{"generic private key", "-----BEGIN PRIVATE KEY-----", true},
		{"public key no match", "-----BEGIN PUBLIC KEY-----", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sec004.Match(tt.input); got != tt.want {
				t.Errorf("SEC004.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStyleCriteria_Count(t *testing.T) {
	cs := StyleCriteria()
	if cs.Len() != 3 {
		t.Errorf("StyleCriteria Len() = %d, want 3", cs.Len())
	}
	for _, c := range cs.All() {
		if c.Category != CategoryStyle {
			t.Errorf("criterion %s has category %s, want style", c.ID, c.Category)
		}
	}
}

func TestStyleCriteria_STY002(t *testing.T) {
	cs := StyleCriteria()
	sty002 := findCriterion(t, cs, "STY002")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exported func", "+func RunQuery(input string) {", true},
		{"unexported func", "+func runQuery(input string) {", false},
		{"exported method", "+func (s *Server) Handle() {", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sty002.Match(tt.input); got != tt.want {
				t.Errorf("STY002.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestStyleCriteria_STY003(t *testing.T) {
	cs := StyleCriteria()
	sty003 := findCriterion(t, cs, "STY003")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"TODO without issue", "// TODO fix this", true},
		{"TODO with issue ref", "// TODO(#123) fix this", false},
		{"TODO with owner", "// TODO(Alice) fix this", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sty003.Match(tt.input); got != tt.want {
				t.Errorf("STY003.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCorrectnessCriteria_Count(t *testing.T) {
	cs := CorrectnessCriteria()
	if cs.Len() != 4 {
		t.Errorf("CorrectnessCriteria Len() = %d, want 4", cs.Len())
	}
	for _, c := range cs.All() {
		if c.Category != CategoryCorrectness {
			t.Errorf("criterion %s has category %s, want correctness", c.ID, c.Category)
		}
	}
}

func TestCorrectnessCriteria_COR003(t *testing.T) {
	cs := CorrectnessCriteria()
	cor003 := findCriterion(t, cs, "COR003")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"panic in added line", `+	panic("fatal error")`, true},
		{"panic in context line", ` 	panic("fatal error")`, false},
		{"no panic", "+	return err", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cor003.Match(tt.input); got != tt.want {
				t.Errorf("COR003.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCorrectnessCriteria_COR004(t *testing.T) {
	cs := CorrectnessCriteria()
	cor004 := findCriterion(t, cs, "COR004")

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"defer in added line", "+\tdefer f.Close()", true},
		{"no defer", "+\tf.Close()", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cor004.Match(tt.input); got != tt.want {
				t.Errorf("COR004.Match(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultCriteria_ContainsAll(t *testing.T) {
	dc := DefaultCriteria()
	sec := SecurityCriteria()
	sty := StyleCriteria()
	cor := CorrectnessCriteria()

	want := sec.Len() + sty.Len() + cor.Len()
	if dc.Len() != want {
		t.Errorf("DefaultCriteria Len() = %d, want %d", dc.Len(), want)
	}
}

func TestSeverityConstants(t *testing.T) {
	if SeverityError != "error" {
		t.Errorf("SeverityError = %q", SeverityError)
	}
	if SeverityWarning != "warning" {
		t.Errorf("SeverityWarning = %q", SeverityWarning)
	}
	if SeverityInfo != "info" {
		t.Errorf("SeverityInfo = %q", SeverityInfo)
	}
}

func TestCategoryConstants(t *testing.T) {
	if CategorySecurity != "security" {
		t.Errorf("CategorySecurity = %q", CategorySecurity)
	}
	if CategoryStyle != "style" {
		t.Errorf("CategoryStyle = %q", CategoryStyle)
	}
	if CategoryCorrectness != "correctness" {
		t.Errorf("CategoryCorrectness = %q", CategoryCorrectness)
	}
}

// findCriterion is a test helper that finds a criterion by ID or fails.
func findCriterion(t *testing.T, cs *CriteriaSet, id string) *Criterion {
	t.Helper()
	for _, c := range cs.All() {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("criterion %s not found", id)
	return nil
}
