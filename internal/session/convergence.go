package session

// detectConvergence checks recent iterations for patterns indicating the loop
// should stop. Returns true and a reason string if convergence is detected.
func detectConvergence(iterations []LoopIteration) (bool, string) {
	if len(iterations) < 2 {
		return false, ""
	}

	// Check 1: No-changes convergence
	// If the last 2 completed (status "idle") iterations produced no file changes,
	// the loop is not making progress.
	if noChangesConverged(iterations) {
		return true, "no changes produced in last 2 iterations"
	}

	// Check 2: Repeating error convergence
	// If the last 3 iterations all failed with the same error (first 100 chars),
	// the loop is stuck on the same problem.
	if reason, ok := repeatingErrorConverged(iterations); ok {
		return true, reason
	}

	// Check 3: Same task convergence
	// If the planner produces the same task title 3 times in a row,
	// it's stuck in a planning loop.
	if sameTaskConverged(iterations) {
		return true, "same task repeated 3 times"
	}

	return false, ""
}

func noChangesConverged(iterations []LoopIteration) bool {
	// Find last 2 completed iterations (status == "idle")
	completed := 0
	for i := len(iterations) - 1; i >= 0 && completed < 2; i-- {
		iter := iterations[i]
		if iter.Status != "idle" {
			continue
		}
		// Check if acceptance result shows no changes, or if no files were changed
		// Use Acceptance.SafePaths + Acceptance.ReviewPaths to determine if changes existed
		hasChanges := false
		if iter.Acceptance != nil {
			hasChanges = len(iter.Acceptance.SafePaths) > 0 || len(iter.Acceptance.ReviewPaths) > 0
		}
		if hasChanges {
			return false
		}
		completed++
	}
	return completed >= 2
}

func repeatingErrorConverged(iterations []LoopIteration) (string, bool) {
	if len(iterations) < 3 {
		return "", false
	}
	// Check last 3 iterations
	last3 := iterations[len(iterations)-3:]

	// All must be failed
	for _, iter := range last3 {
		if iter.Status != "failed" || iter.Error == "" {
			return "", false
		}
	}

	// Compare error prefixes (first 100 chars)
	prefix := errorPrefix(last3[0].Error)
	for _, iter := range last3[1:] {
		if errorPrefix(iter.Error) != prefix {
			return "", false
		}
	}

	return "repeating error: " + prefix, true
}

func errorPrefix(s string) string {
	if len(s) > 100 {
		return s[:100]
	}
	return s
}

func sameTaskConverged(iterations []LoopIteration) bool {
	if len(iterations) < 3 {
		return false
	}
	last3 := iterations[len(iterations)-3:]

	// All must have non-empty task titles
	title := last3[0].Task.Title
	if title == "" {
		return false
	}
	for _, iter := range last3[1:] {
		if iter.Task.Title != title {
			return false
		}
	}
	return true
}
