package session

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

func TestDecomposeToSprints(t *testing.T) {
	rm := &roadmap.Roadmap{
		Title: "Test Roadmap",
		Phases: []roadmap.Phase{
			{
				Name: "Phase 1",
				Sections: []roadmap.Section{
					{
						Name: "Section A",
						Tasks: []roadmap.Task{
							{ID: "1.1", Description: "Task one `P1` `S`", Done: false},
							{ID: "1.2", Description: "Task two `P2` `M`", Done: true},
							{ID: "1.3", Description: "Task three `P1` `L`", Done: false},
							{ID: "1.4", Description: "Architecture review `P1` `L`", Done: false},
						},
					},
				},
			},
		},
	}

	units := DecomposeToSprints(rm, 10)
	if len(units) != 3 {
		t.Fatalf("expected 3 units (1 done), got %d", len(units))
	}

	// Sorted by budget: S first.
	if units[0].Size != "S" {
		t.Errorf("expected first unit to be S, got %s", units[0].Size)
	}
	if units[0].Provider != "gemini" {
		t.Errorf("expected S/P1 → gemini, got %s", units[0].Provider)
	}
	if units[1].Size != "L" {
		t.Errorf("expected second unit to be L, got %s", units[1].Size)
	}
	if units[1].Provider != string(DefaultPrimaryProvider()) {
		t.Errorf("expected generic L/P1 → %s, got %s", DefaultPrimaryProvider(), units[1].Provider)
	}
	if units[2].Provider != "claude" {
		t.Errorf("expected architecture L/P1 → claude, got %s", units[2].Provider)
	}
}

func TestDecomposeMaxUnits(t *testing.T) {
	rm := &roadmap.Roadmap{
		Phases: []roadmap.Phase{
			{
				Sections: []roadmap.Section{
					{
						Tasks: []roadmap.Task{
							{ID: "1", Description: "A `S`", Done: false},
							{ID: "2", Description: "B `S`", Done: false},
							{ID: "3", Description: "C `S`", Done: false},
						},
					},
				},
			},
		},
	}

	units := DecomposeToSprints(rm, 2)
	if len(units) != 2 {
		t.Fatalf("expected max 2 units, got %d", len(units))
	}
}

func TestFilterParallelizable(t *testing.T) {
	units := []SprintUnit{
		{ID: "1", DependsOn: nil},
		{ID: "2", DependsOn: []string{"1"}},
		{ID: "3", DependsOn: nil},
		{ID: "4", DependsOn: []string{"99"}}, // unmet dep
	}

	parallel := FilterParallelizable(units)
	// IDs 1 and 3 have no deps, 2 depends on 1 (which is "done" after processing), 4 depends on 99 (unmet).
	if len(parallel) != 3 {
		t.Fatalf("expected 3 parallelizable units, got %d", len(parallel))
	}
}

func TestDecomposeToSprintsEdgeCases(t *testing.T) {
	defaultProvider := string(DefaultPrimaryProvider())

	cases := []struct {
		name      string
		rm        *roadmap.Roadmap
		maxUnits  int
		wantCount int
		check     func(t *testing.T, units []SprintUnit)
	}{
		{
			name:      "empty roadmap",
			rm:        &roadmap.Roadmap{},
			maxUnits:  10,
			wantCount: 0,
			check:     nil,
		},
		{
			name: "all tasks done",
			rm: &roadmap.Roadmap{
				Phases: []roadmap.Phase{{
					Name: "Phase 1",
					Sections: []roadmap.Section{{
						Name: "S",
						Tasks: []roadmap.Task{
							{ID: "1", Description: "done task", Done: true},
							{ID: "2", Description: "also done", Done: true},
						},
					}},
				}},
			},
			maxUnits:  10,
			wantCount: 0,
			check:     nil,
		},
		{
			name: "single-item sprint",
			rm: &roadmap.Roadmap{
				Phases: []roadmap.Phase{{
					Name: "Phase 1",
					Sections: []roadmap.Section{{
						Name: "S",
						Tasks: []roadmap.Task{
							{ID: "only", Description: "The only task `M`", Done: false},
						},
					}},
				}},
			},
			maxUnits:  10,
			wantCount: 1,
			check: func(t *testing.T, units []SprintUnit) {
				t.Helper()
				u := units[0]
				if u.ID != "only" {
					t.Errorf("expected id=only, got %s", u.ID)
				}
				if u.Size != "M" {
					t.Errorf("expected size=M, got %s", u.Size)
				}
				if u.BudgetUSD != 2.0 {
					t.Errorf("expected budget=2.0, got %f", u.BudgetUSD)
				}
				if u.Provider != defaultProvider {
					t.Errorf("expected provider=%s, got %s", defaultProvider, u.Provider)
				}
			},
		},
		{
			name: "P3 priority uses default routing",
			rm: &roadmap.Roadmap{
				Phases: []roadmap.Phase{{
					Name: "Phase 1",
					Sections: []roadmap.Section{{
						Name: "S",
						Tasks: []roadmap.Task{
							{ID: "p3-s", Description: "Small P3 task `P3` `S`", Done: false},
							{ID: "p3-m", Description: "Medium P3 task `P3`", Done: false},
							{ID: "p3-l", Description: "Large P3 task `P3` `L`", Done: false},
						},
					}},
				}},
			},
			maxUnits:  10,
			wantCount: 3,
			check: func(t *testing.T, units []SprintUnit) {
				t.Helper()
				byID := make(map[string]SprintUnit, len(units))
				for _, u := range units {
					byID[u.ID] = u
				}
				// S tag → gemini (size-based, not priority-based)
				if byID["p3-s"].Provider != "gemini" {
					t.Errorf("P3 S task: expected provider=gemini, got %s", byID["p3-s"].Provider)
				}
				// M with P3 (no P2) → default provider
				if byID["p3-m"].Provider != defaultProvider {
					t.Errorf("P3 M task: expected provider=%s, got %s", defaultProvider, byID["p3-m"].Provider)
				}
				// L with P3 (no architecture keywords) → default provider
				if byID["p3-l"].Provider != defaultProvider {
					t.Errorf("P3 L task: expected provider=%s, got %s", defaultProvider, byID["p3-l"].Provider)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			units := DecomposeToSprints(tc.rm, tc.maxUnits)
			if len(units) != tc.wantCount {
				t.Fatalf("expected %d units, got %d", tc.wantCount, len(units))
			}
			if tc.check != nil {
				tc.check(t, units)
			}
		})
	}
}
