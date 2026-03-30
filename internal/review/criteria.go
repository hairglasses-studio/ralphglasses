package review

import "regexp"

// Severity indicates how serious a review finding is.
type Severity string

const (
	SeverityError   Severity = "error"   // Must fix before merge
	SeverityWarning Severity = "warning" // Should fix, not blocking
	SeverityInfo    Severity = "info"    // Suggestion or style nit
)

// Category groups review criteria by domain.
type Category string

const (
	CategorySecurity    Category = "security"
	CategoryStyle       Category = "style"
	CategoryCorrectness Category = "correctness"
)

// Criterion is a single review rule that matches against diff content.
type Criterion struct {
	ID       string   // Unique identifier, e.g. "SEC001"
	Name     string   // Human-readable name
	Category Category // Domain grouping
	Severity Severity // Default severity for matches
	Pattern  *regexp.Regexp
	Message  string // Description shown in findings
}

// Match tests whether a line of diff content triggers this criterion.
// Returns true if the pattern matches the line.
func (c *Criterion) Match(line string) bool {
	return c.Pattern.MatchString(line)
}

// CriteriaSet is a composable collection of review criteria.
type CriteriaSet struct {
	criteria []*Criterion
}

// NewCriteriaSet creates an empty criteria set.
func NewCriteriaSet() *CriteriaSet {
	return &CriteriaSet{}
}

// Add appends one or more criteria to the set.
func (cs *CriteriaSet) Add(criteria ...*Criterion) {
	cs.criteria = append(cs.criteria, criteria...)
}

// Merge combines another criteria set into this one.
func (cs *CriteriaSet) Merge(other *CriteriaSet) {
	cs.criteria = append(cs.criteria, other.criteria...)
}

// All returns all criteria in the set.
func (cs *CriteriaSet) All() []*Criterion {
	out := make([]*Criterion, len(cs.criteria))
	copy(out, cs.criteria)
	return out
}

// ByCategory returns criteria filtered to a specific category.
func (cs *CriteriaSet) ByCategory(cat Category) []*Criterion {
	var out []*Criterion
	for _, c := range cs.criteria {
		if c.Category == cat {
			out = append(out, c)
		}
	}
	return out
}

// Len returns the number of criteria in the set.
func (cs *CriteriaSet) Len() int {
	return len(cs.criteria)
}

// SecurityCriteria returns built-in security rules.
func SecurityCriteria() *CriteriaSet {
	cs := NewCriteriaSet()
	cs.Add(
		&Criterion{
			ID:       "SEC001",
			Name:     "hardcoded-secret",
			Category: CategorySecurity,
			Severity: SeverityError,
			Pattern:  regexp.MustCompile(`(?i)(password|secret|api[_-]?key|token)\s*[:=]\s*["'][\w./$@!%^&*()+-]{8,}["']`),
			Message:  "Possible hardcoded secret or credential. Use environment variables or a secrets manager.",
		},
		&Criterion{
			ID:       "SEC002",
			Name:     "sql-injection",
			Category: CategorySecurity,
			Severity: SeverityError,
			Pattern:  regexp.MustCompile(`(?i)(fmt\.Sprintf|"[^"]*\+[^"]*")\s*.*(?:SELECT|INSERT|UPDATE|DELETE|DROP|ALTER)\b`),
			Message:  "Possible SQL injection via string concatenation. Use parameterized queries.",
		},
		&Criterion{
			ID:       "SEC003",
			Name:     "sql-string-concat",
			Category: CategorySecurity,
			Severity: SeverityWarning,
			Pattern:  regexp.MustCompile(`(?i)(?:SELECT|INSERT|UPDATE|DELETE)\s.*["']\s*\+\s*\w+`),
			Message:  "SQL query built with string concatenation. Prefer parameterized queries.",
		},
		&Criterion{
			ID:       "SEC004",
			Name:     "private-key-literal",
			Category: CategorySecurity,
			Severity: SeverityError,
			Pattern:  regexp.MustCompile(`-----BEGIN\s+(RSA\s+)?PRIVATE\s+KEY-----`),
			Message:  "Private key embedded in source. Store keys in a secure vault.",
		},
	)
	return cs
}

// StyleCriteria returns built-in code style rules.
func StyleCriteria() *CriteriaSet {
	cs := NewCriteriaSet()
	cs.Add(
		&Criterion{
			ID:       "STY001",
			Name:     "long-function",
			Category: CategoryStyle,
			Severity: SeverityInfo,
			// Matches a "func" definition — the agent counts lines separately.
			// This criterion is handled specially in the agent via line counting.
			Pattern:  regexp.MustCompile(`^[+]func\s+`),
			Message:  "Function exceeds recommended length. Consider splitting into smaller functions.",
		},
		&Criterion{
			ID:       "STY002",
			Name:     "missing-doc-comment",
			Category: CategoryStyle,
			Severity: SeverityInfo,
			// Exported function without preceding doc comment — detected
			// when a line starting with "func" and an uppercase name is added.
			Pattern:  regexp.MustCompile(`^[+]func\s+([A-Z]|.*\)\s+[A-Z])`),
			Message:  "Exported function missing doc comment.",
		},
		&Criterion{
			ID:       "STY003",
			Name:     "todo-without-issue",
			Category: CategoryStyle,
			Severity: SeverityInfo,
			Pattern:  regexp.MustCompile(`(?i)//\s*TODO\s*[^(#A-Z]`),
			Message:  "TODO comment without an associated issue or owner. Add a tracking reference.",
		},
	)
	return cs
}

// CorrectnessCriteria returns built-in correctness rules.
func CorrectnessCriteria() *CriteriaSet {
	cs := NewCriteriaSet()
	cs.Add(
		&Criterion{
			ID:       "COR001",
			Name:     "unchecked-error",
			Category: CategoryCorrectness,
			Severity: SeverityWarning,
			// Matches common Go pattern of assigning to _ for error return.
			Pattern:  regexp.MustCompile(`^[+].*\b_\s*,?\s*:?=\s*\w+.*\(.*\)\s*$`),
			Message:  "Error return value appears to be discarded. Check and handle the error.",
		},
		&Criterion{
			ID:       "COR002",
			Name:     "nil-pointer-deref",
			Category: CategoryCorrectness,
			Severity: SeverityWarning,
			// Method call on result of function that might return nil, without nil check.
			Pattern:  regexp.MustCompile(`^[+].*\w+\(.*\)\.\w+`),
			Message:  "Calling method on function result without nil check. Verify the return is non-nil.",
		},
		&Criterion{
			ID:       "COR003",
			Name:     "panic-in-library",
			Category: CategoryCorrectness,
			Severity: SeverityError,
			Pattern:  regexp.MustCompile(`^[+].*\bpanic\(`),
			Message:  "panic() in library code. Return an error instead of panicking.",
		},
		&Criterion{
			ID:       "COR004",
			Name:     "defer-in-loop",
			Category: CategoryCorrectness,
			Severity: SeverityWarning,
			Pattern:  regexp.MustCompile(`^[+]\s*defer\s+`),
			Message:  "defer inside a loop can cause resource leaks. Consider restructuring.",
		},
	)
	return cs
}

// DefaultCriteria returns a CriteriaSet containing all built-in rules.
func DefaultCriteria() *CriteriaSet {
	cs := NewCriteriaSet()
	cs.Merge(SecurityCriteria())
	cs.Merge(StyleCriteria())
	cs.Merge(CorrectnessCriteria())
	return cs
}
