package session

import "fmt"

// TeamSafetyConfig defines safety limits for team operations at autonomy levels 2-3.
type TeamSafetyConfig struct {
	MaxNestingDepth int // max team-within-team depth, default 3
	MaxTeamSize     int // max members per team, default 10
	MaxTotalTeams   int // fleet-wide team limit, default 20
}

// DefaultTeamSafety provides conservative defaults for team safety.
var DefaultTeamSafety = TeamSafetyConfig{
	MaxNestingDepth: 3,
	MaxTeamSize:     10,
	MaxTotalTeams:   20,
}

// TeamSafetyError is returned when a team safety check fails.
type TeamSafetyError struct {
	Check   string // which check failed
	Message string // human-readable explanation
}

func (e *TeamSafetyError) Error() string {
	return fmt.Sprintf("team safety: %s: %s", e.Check, e.Message)
}

// ValidateTeamCreate checks whether a new team can be created given the
// current fleet state. It enforces team size and fleet-wide team count limits.
func ValidateTeamCreate(name string, memberCount int, existingTeams int, config TeamSafetyConfig) error {
	if config.MaxTotalTeams > 0 && existingTeams >= config.MaxTotalTeams {
		return &TeamSafetyError{
			Check:   "max_total_teams",
			Message: fmt.Sprintf("fleet already has %d teams (limit: %d)", existingTeams, config.MaxTotalTeams),
		}
	}

	if config.MaxTeamSize > 0 && memberCount > config.MaxTeamSize {
		return &TeamSafetyError{
			Check:   "max_team_size",
			Message: fmt.Sprintf("team %q requests %d members, exceeding limit of %d", name, memberCount, config.MaxTeamSize),
		}
	}

	return nil
}

// ValidateTeamNesting checks that creating a sub-team won't exceed the
// maximum nesting depth. parentDepth is the depth of the parent team
// (0 for top-level teams).
func ValidateTeamNesting(parentDepth int, config TeamSafetyConfig) error {
	if config.MaxNestingDepth > 0 && parentDepth+1 > config.MaxNestingDepth {
		return &TeamSafetyError{
			Check:   "max_nesting_depth",
			Message: fmt.Sprintf("creating sub-team at depth %d would exceed max nesting depth of %d", parentDepth+1, config.MaxNestingDepth),
		}
	}
	return nil
}
