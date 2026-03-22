package awesome

// Diff computes new and removed entries between a previous and current index.
func Diff(prev, current *Index) *DiffResult {
	result := &DiffResult{}

	if prev == nil {
		result.New = current.Entries
		return result
	}

	prevURLs := make(map[string]AwesomeEntry, len(prev.Entries))
	for _, e := range prev.Entries {
		prevURLs[e.URL] = e
	}

	currURLs := make(map[string]struct{}, len(current.Entries))
	for _, e := range current.Entries {
		currURLs[e.URL] = struct{}{}
		if _, ok := prevURLs[e.URL]; !ok {
			result.New = append(result.New, e)
		}
	}

	for _, e := range prev.Entries {
		if _, ok := currURLs[e.URL]; !ok {
			result.Removed = append(result.Removed, e)
		}
	}

	return result
}
