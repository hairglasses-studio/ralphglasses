package session

// CostThreshold is the estimated task complexity below which a cheaper model is suggested.
const CostThreshold = 0.50 // USD

// SuggestCheaperModel checks if the task budget is below the threshold and
// returns a cheaper alternative model, if available.
// Returns the original model and false if no cheaper option makes sense.
func SuggestCheaperModel(provider Provider, currentModel string, estimatedBudget float64) (string, bool) {
	if estimatedBudget > CostThreshold {
		return currentModel, false
	}

	cheap := CheapestModel(provider)
	if cheap == nil || cheap.ID == currentModel {
		return currentModel, false
	}

	// Only suggest if the cheaper model is actually cheaper
	current := LookupModel(currentModel)
	if current == nil {
		return currentModel, false
	}
	if cheap.CostPerMTokIn >= current.CostPerMTokIn {
		return currentModel, false
	}

	return cheap.ID, true
}
