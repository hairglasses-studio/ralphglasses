package fleet

import (
	"testing"
)

func TestMatchSkills_NoRequirements(t *testing.T) {
	card := &AgentCard{}
	// No required skills → always score 1.
	got := matchSkills(card, nil)
	if got != 1 {
		t.Errorf("matchSkills(no requirements) = %d, want 1", got)
	}
}

func TestMatchSkills_BySkillID(t *testing.T) {
	card := &AgentCard{
		Skills: []AgentSkill{
			{ID: "CodeReview", Tags: []string{}},
		},
	}
	got := matchSkills(card, []string{"codereview"})
	if got != 1 {
		t.Errorf("matchSkills by skill ID = %d, want 1", got)
	}
}

func TestMatchSkills_BySkillTag(t *testing.T) {
	card := &AgentCard{
		Skills: []AgentSkill{
			{ID: "analysis", Tags: []string{"Go", "Python"}},
		},
	}
	got := matchSkills(card, []string{"go", "python"})
	if got != 2 {
		t.Errorf("matchSkills by skill tags = %d, want 2", got)
	}
}

func TestMatchSkills_ByCardTag(t *testing.T) {
	card := &AgentCard{
		Tags: []string{"backend", "infra"},
	}
	got := matchSkills(card, []string{"backend"})
	if got != 1 {
		t.Errorf("matchSkills by card tag = %d, want 1", got)
	}
}

func TestMatchSkills_NoMatch(t *testing.T) {
	card := &AgentCard{
		Skills: []AgentSkill{{ID: "frontend"}},
	}
	got := matchSkills(card, []string{"backend"})
	if got != 0 {
		t.Errorf("matchSkills no match = %d, want 0", got)
	}
}

func TestSkillMatches_CaseInsensitive(t *testing.T) {
	card := &AgentCard{
		Skills: []AgentSkill{{ID: "GoLang"}},
	}
	if !skillMatches(card, "golang") {
		t.Error("skillMatches should match case-insensitively")
	}
}

func TestSkillMatches_NoMatch(t *testing.T) {
	card := &AgentCard{
		Skills: []AgentSkill{{ID: "python"}},
		Tags:   []string{"backend"},
	}
	if skillMatches(card, "golang") {
		t.Error("skillMatches should return false for non-matching skill")
	}
}

func TestSyncKey(t *testing.T) {
	got := syncKey("node1", "myrepo")
	want := "node1:myrepo"
	if got != want {
		t.Errorf("syncKey = %q, want %q", got, want)
	}
}

func TestSyncKey_EmptyParts(t *testing.T) {
	got := syncKey("", "")
	if got != ":" {
		t.Errorf("syncKey(\"\", \"\") = %q, want \":\"", got)
	}
}
