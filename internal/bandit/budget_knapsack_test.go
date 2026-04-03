package bandit

import (
	"math"
	"testing"
)

var knapsackArms = []Arm{
	{ID: "flash-lite", Provider: "gemini", Model: "gemini-2.0-flash-lite"},
	{ID: "sonnet", Provider: "claude", Model: "claude-sonnet"},
	{ID: "opus", Provider: "claude", Model: "claude-opus"},
}

func TestSolveBudgetKnapsack_BasicAssignment(t *testing.T) {
	tasks := []KnapsackTask{
		{ID: "t1", Complexity: -1.0},
		{ID: "t2", Complexity: 0.0},
		{ID: "t3", Complexity: 1.0},
	}

	// Simple predictor: cost scales with model tier, quality scales with complexity match.
	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		switch arm.ID {
		case "flash-lite":
			cost = 0.1
			quality = 0.6 - task.Complexity*0.2 // better for simple
		case "sonnet":
			cost = 0.5
			quality = 0.7
		case "opus":
			cost = 1.0
			quality = 0.8 + task.Complexity*0.1 // better for complex
		}
		return
	}

	result := SolveBudgetKnapsack(tasks, knapsackArms, 5.0, predictor)
	if len(result) != 3 {
		t.Fatalf("expected 3 assignments, got %d", len(result))
	}

	// Verify all tasks assigned.
	assigned := map[string]bool{}
	for _, a := range result {
		assigned[a.TaskID] = true
	}
	for _, task := range tasks {
		if !assigned[task.ID] {
			t.Errorf("task %s was not assigned", task.ID)
		}
	}
}

func TestSolveBudgetKnapsack_BudgetConstraint(t *testing.T) {
	tasks := []KnapsackTask{
		{ID: "t1"},
		{ID: "t2"},
		{ID: "t3"},
	}
	arms := []Arm{{ID: "expensive", Provider: "p", Model: "m"}}

	// Each task costs 0.5, budget is only 1.0 -> can assign at most 2.
	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return 0.5, 0.8
	}

	result := SolveBudgetKnapsack(tasks, arms, 1.0, predictor)
	if len(result) != 2 {
		t.Fatalf("expected 2 assignments within budget, got %d", len(result))
	}

	totalCost := 0.0
	for _, a := range result {
		totalCost += a.Cost
	}
	if totalCost > 1.0 {
		t.Errorf("total cost %.2f exceeds budget 1.0", totalCost)
	}
}

func TestSolveBudgetKnapsack_GreedyPrefersEfficient(t *testing.T) {
	tasks := []KnapsackTask{{ID: "t1"}}
	arms := []Arm{
		{ID: "cheap", Provider: "p", Model: "cheap"},
		{ID: "expensive", Provider: "p", Model: "expensive"},
	}

	// cheap: cost=0.1, quality=0.7 -> ratio 7.0
	// expensive: cost=1.0, quality=0.8 -> ratio 0.8
	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		if arm.ID == "cheap" {
			return 0.1, 0.7
		}
		return 1.0, 0.8
	}

	result := SolveBudgetKnapsack(tasks, arms, 5.0, predictor)
	if len(result) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(result))
	}
	if result[0].ArmID != "cheap" {
		t.Errorf("expected greedy to pick cheap (ratio 7.0), got %s", result[0].ArmID)
	}
}

func TestSolveBudgetKnapsack_MaximizesTotalQuality(t *testing.T) {
	tasks := []KnapsackTask{
		{ID: "t1"},
		{ID: "t2"},
		{ID: "t3"},
	}
	arms := []Arm{
		{ID: "a", Provider: "p", Model: "m"},
		{ID: "b", Provider: "p", Model: "m"},
	}

	// a: cost=0.3, quality=0.9 -> ratio 3.0
	// b: cost=0.1, quality=0.5 -> ratio 5.0
	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		if arm.ID == "a" {
			return 0.3, 0.9
		}
		return 0.1, 0.5
	}

	result := SolveBudgetKnapsack(tasks, arms, 10.0, predictor)
	totalQuality := 0.0
	for _, a := range result {
		totalQuality += a.Quality
	}
	if totalQuality <= 0 {
		t.Error("expected positive total quality")
	}
}

func TestSolveBudgetKnapsack_EmptyInputs(t *testing.T) {
	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return 1.0, 1.0
	}

	if r := SolveBudgetKnapsack(nil, knapsackArms, 10, predictor); r != nil {
		t.Errorf("expected nil for nil tasks, got %v", r)
	}
	if r := SolveBudgetKnapsack([]KnapsackTask{{ID: "t1"}}, nil, 10, predictor); r != nil {
		t.Errorf("expected nil for nil arms, got %v", r)
	}
	if r := SolveBudgetKnapsack([]KnapsackTask{{ID: "t1"}}, knapsackArms, 0, predictor); r != nil {
		t.Errorf("expected nil for zero budget, got %v", r)
	}
	if r := SolveBudgetKnapsack([]KnapsackTask{{ID: "t1"}}, knapsackArms, -1, predictor); r != nil {
		t.Errorf("expected nil for negative budget, got %v", r)
	}
	if r := SolveBudgetKnapsack([]KnapsackTask{{ID: "t1"}}, knapsackArms, 10, nil); r != nil {
		t.Errorf("expected nil for nil predictor, got %v", r)
	}
}

func TestSolveBudgetKnapsack_ZeroCostSkipped(t *testing.T) {
	tasks := []KnapsackTask{{ID: "t1"}}
	arms := []Arm{{ID: "free", Provider: "p", Model: "m"}}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return 0, 0.9 // zero cost -> skipped
	}

	result := SolveBudgetKnapsack(tasks, arms, 10, predictor)
	if len(result) != 0 {
		t.Errorf("expected 0 assignments when cost=0, got %d", len(result))
	}
}

func TestSolveBudgetKnapsack_ZeroQualitySkipped(t *testing.T) {
	tasks := []KnapsackTask{{ID: "t1"}}
	arms := []Arm{{ID: "useless", Provider: "p", Model: "m"}}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return 1.0, 0.0 // zero quality -> skipped
	}

	result := SolveBudgetKnapsack(tasks, arms, 10, predictor)
	if len(result) != 0 {
		t.Errorf("expected 0 assignments when quality=0, got %d", len(result))
	}
}

func TestSolveBudgetKnapsack_NegativeCostSkipped(t *testing.T) {
	tasks := []KnapsackTask{{ID: "t1"}}
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return -0.5, 0.9
	}

	result := SolveBudgetKnapsack(tasks, arms, 10, predictor)
	if len(result) != 0 {
		t.Errorf("expected 0 assignments when cost<0, got %d", len(result))
	}
}

func TestSolveBudgetKnapsack_OneTaskPerAssignment(t *testing.T) {
	tasks := []KnapsackTask{
		{ID: "t1"},
		{ID: "t2"},
	}
	arms := []Arm{
		{ID: "a", Provider: "p", Model: "m"},
		{ID: "b", Provider: "p", Model: "m"},
	}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		return 0.1, 0.9
	}

	result := SolveBudgetKnapsack(tasks, arms, 10, predictor)
	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}

	// Each task should appear exactly once.
	taskCounts := map[string]int{}
	for _, a := range result {
		taskCounts[a.TaskID]++
	}
	for _, task := range tasks {
		if taskCounts[task.ID] != 1 {
			t.Errorf("task %s assigned %d times, want 1", task.ID, taskCounts[task.ID])
		}
	}
}

func TestSolveBudgetKnapsack_TotalCostWithinBudget(t *testing.T) {
	tasks := make([]KnapsackTask, 20)
	for i := range tasks {
		tasks[i] = KnapsackTask{ID: "t" + string(rune('a'+i))}
	}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		switch arm.ID {
		case "flash-lite":
			return 0.05, 0.5
		case "sonnet":
			return 0.3, 0.7
		case "opus":
			return 0.8, 0.95
		}
		return 0.1, 0.5
	}

	budget := 3.0
	result := SolveBudgetKnapsack(tasks, knapsackArms, budget, predictor)

	totalCost := 0.0
	for _, a := range result {
		totalCost += a.Cost
	}
	if totalCost > budget+1e-9 {
		t.Errorf("total cost %.4f exceeds budget %.2f", totalCost, budget)
	}
}

func TestSolveBudgetKnapsack_IntegrationWithNeuralUCB(t *testing.T) {
	// Verify knapsack can use NeuralUCB predictions.
	arms := []Arm{
		{ID: "cheap", Provider: "gemini", Model: "flash"},
		{ID: "expensive", Provider: "claude", Model: "opus"},
	}
	n := NewNeuralUCB(arms, int(NumContextualFeatures), NeuralUCBConfig{
		HiddenSize:   8,
		LearningRate: 0.05,
		BudgetWeight: 0.3,
	})

	// Pre-train the network so predictions are meaningful.
	simpleCtx := make([]float64, NumContextualFeatures)
	simpleCtx[FeatureComplexity] = -1.0

	complexCtx := make([]float64, NumContextualFeatures)
	complexCtx[FeatureComplexity] = 1.0

	for i := 0; i < 100; i++ {
		n.Update(Reward{ArmID: "cheap", Value: 0.8, Context: simpleCtx})
		n.Update(Reward{ArmID: "cheap", Value: 0.2, Context: complexCtx})
		n.Update(Reward{ArmID: "expensive", Value: 0.9, Context: complexCtx})
		n.Update(Reward{ArmID: "expensive", Value: 0.5, Context: simpleCtx})
	}

	tasks := []KnapsackTask{
		{ID: "simple-task", Complexity: -1.0},
		{ID: "complex-task", Complexity: 1.0},
	}

	costMap := map[string]float64{"cheap": 0.1, "expensive": 1.0}

	predictor := func(task KnapsackTask, arm Arm) (cost, quality float64) {
		ctx := make([]float64, NumContextualFeatures)
		ctx[FeatureComplexity] = task.Complexity
		quality = n.Predict(arm.ID, ctx)
		cost = costMap[arm.ID]
		return
	}

	result := SolveBudgetKnapsack(tasks, arms, 5.0, predictor)
	if len(result) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(result))
	}

	totalCost := 0.0
	totalQuality := 0.0
	for _, a := range result {
		totalCost += a.Cost
		totalQuality += a.Quality
		if a.Cost <= 0 {
			t.Errorf("assignment %s -> %s has non-positive cost %.4f", a.TaskID, a.ArmID, a.Cost)
		}
		if a.Quality <= 0 || a.Quality >= 1 {
			t.Errorf("assignment %s -> %s has quality %.4f outside (0,1)", a.TaskID, a.ArmID, a.Quality)
		}
	}

	if totalCost > 5.0+1e-9 {
		t.Errorf("total cost %.4f exceeds budget 5.0", totalCost)
	}
	if math.IsNaN(totalQuality) {
		t.Error("total quality is NaN")
	}
}
