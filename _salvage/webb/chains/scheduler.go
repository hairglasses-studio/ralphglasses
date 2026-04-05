package chains

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled and event-triggered chain executions
type Scheduler struct {
	executor *Executor
	registry *Registry
	cron     *cron.Cron
	mu       sync.RWMutex
	jobs     map[string]cron.EntryID
	events   map[string][]string // event name -> chain names
	running  bool
}

// NewScheduler creates a new chain scheduler
func NewScheduler(executor *Executor, registry *Registry) *Scheduler {
	return &Scheduler{
		executor: executor,
		registry: registry,
		cron:     cron.New(cron.WithSeconds()),
		jobs:     make(map[string]cron.EntryID),
		events:   make(map[string][]string),
	}
}

// Start begins the scheduler
func (s *Scheduler) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler already running")
	}

	// Register all scheduled chains
	for _, name := range s.registry.ListByTrigger(TriggerScheduled) {
		chain, err := s.registry.Get(name)
		if err != nil {
			continue
		}

		if chain.Trigger.Cron != "" {
			if err := s.addCronJob(chain); err != nil {
				log.Printf("[chains] Failed to schedule %s: %v", name, err)
			}
		}
	}

	// Register all event-triggered chains
	for _, name := range s.registry.ListByTrigger(TriggerEvent) {
		chain, err := s.registry.Get(name)
		if err != nil {
			continue
		}

		if chain.Trigger.Event != "" {
			s.events[chain.Trigger.Event] = append(s.events[chain.Trigger.Event], name)
		}
	}

	s.cron.Start()
	s.running = true

	log.Printf("[chains] Scheduler started with %d cron jobs and %d event triggers",
		len(s.jobs), len(s.events))

	return nil
}

// Stop halts the scheduler
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	ctx := s.cron.Stop()
	<-ctx.Done()
	s.running = false

	log.Printf("[chains] Scheduler stopped")
}

func (s *Scheduler) addCronJob(chain *ChainDefinition) error {
	entryID, err := s.cron.AddFunc(chain.Trigger.Cron, func() {
		s.executeChain(chain.Name, "schedule:"+chain.Trigger.Cron, nil)
	})
	if err != nil {
		return err
	}

	s.jobs[chain.Name] = entryID
	log.Printf("[chains] Scheduled %s with cron: %s", chain.Name, chain.Trigger.Cron)
	return nil
}

// TriggerEvent triggers all chains listening for a specific event
func (s *Scheduler) TriggerEvent(event string, data map[string]interface{}) error {
	s.mu.RLock()
	chainNames := s.events[event]
	s.mu.RUnlock()

	if len(chainNames) == 0 {
		return nil // No chains registered for this event
	}

	for _, name := range chainNames {
		chain, err := s.registry.Get(name)
		if err != nil {
			continue
		}

		// Check filter if specified
		if chain.Trigger.Filter != "" {
			// Simple filter evaluation - could be expanded
			if !s.evaluateFilter(chain.Trigger.Filter, data) {
				continue
			}
		}

		go s.executeChain(name, "event:"+event, data)
	}

	return nil
}

func (s *Scheduler) evaluateFilter(filter string, data map[string]interface{}) bool {
	// Interpolate the filter with event data
	interpolated := s.interpolateFilter(filter, data)
	interpolated = strings.TrimSpace(interpolated)

	// Evaluate the expression
	result, ok := s.evaluateExpression(interpolated)
	if ok {
		return result
	}

	// If we couldn't parse it, default to true (allow the chain to run)
	return true
}

// interpolateFilter replaces {{ variable }} patterns with data values
func (s *Scheduler) interpolateFilter(filter string, data map[string]interface{}) string {
	re := regexp.MustCompile(`\{\{\s*([^}]+)\s*\}\}`)
	return re.ReplaceAllStringFunc(filter, func(match string) string {
		path := strings.TrimSpace(match[2 : len(match)-2])
		if val := s.getNestedValue(data, path); val != nil {
			return fmt.Sprintf("%v", val)
		}
		return match
	})
}

// getNestedValue retrieves a value from a nested map using dot notation
func (s *Scheduler) getNestedValue(data map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	current := interface{}(data)

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			val, exists := v[part]
			if !exists {
				return nil
			}
			current = val
		case map[string]string:
			val, exists := v[part]
			if !exists {
				return nil
			}
			return val
		default:
			return nil
		}
	}

	return current
}

// evaluateExpression evaluates filter expressions with operators
func (s *Scheduler) evaluateExpression(expr string) (bool, bool) {
	expr = strings.TrimSpace(expr)

	// Handle logical operators
	if idx := strings.Index(strings.ToLower(expr), " or "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+4:]
		leftResult, leftOk := s.evaluateExpression(left)
		rightResult, rightOk := s.evaluateExpression(right)
		if leftOk && rightOk {
			return leftResult || rightResult, true
		}
	}

	if idx := strings.Index(strings.ToLower(expr), " and "); idx != -1 {
		left := expr[:idx]
		right := expr[idx+5:]
		leftResult, leftOk := s.evaluateExpression(left)
		rightResult, rightOk := s.evaluateExpression(right)
		if leftOk && rightOk {
			return leftResult && rightResult, true
		}
	}

	// Handle "not in" operator
	if idx := strings.Index(strings.ToLower(expr), " not in "); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+8:])
		return !s.evaluateIn(left, right), true
	}

	// Handle "in" operator
	if idx := strings.Index(strings.ToLower(expr), " in "); idx != -1 {
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+4:])
		return s.evaluateIn(left, right), true
	}

	// Handle comparison operators
	operators := []struct {
		op   string
		eval func(a, b string) bool
	}{
		{"<=", func(a, b string) bool { return s.compareValues(a, b) <= 0 }},
		{">=", func(a, b string) bool { return s.compareValues(a, b) >= 0 }},
		{"!=", func(a, b string) bool { return strings.TrimSpace(a) != strings.TrimSpace(b) }},
		{"==", func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }},
		{"<", func(a, b string) bool { return s.compareValues(a, b) < 0 }},
		{">", func(a, b string) bool { return s.compareValues(a, b) > 0 }},
	}

	for _, op := range operators {
		if idx := strings.Index(expr, op.op); idx != -1 {
			left := strings.TrimSpace(expr[:idx])
			right := strings.TrimSpace(expr[idx+len(op.op):])
			return op.eval(left, right), true
		}
	}

	// Simple boolean check
	lower := strings.ToLower(expr)
	if lower == "true" || lower == "1" || lower == "yes" {
		return true, true
	}
	if lower == "false" || lower == "0" || lower == "no" || lower == "" {
		return false, true
	}

	return false, false
}

// compareValues compares two values, attempting numeric comparison first
func (s *Scheduler) compareValues(a, b string) int {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)

	var aNum, bNum float64
	_, aErr := fmt.Sscanf(a, "%f", &aNum)
	_, bErr := fmt.Sscanf(b, "%f", &bNum)

	if aErr == nil && bErr == nil {
		if aNum < bNum {
			return -1
		} else if aNum > bNum {
			return 1
		}
		return 0
	}

	return strings.Compare(a, b)
}

// evaluateIn checks if value is in a list
func (s *Scheduler) evaluateIn(value, list string) bool {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "'\"")

	list = strings.TrimSpace(list)
	if strings.HasPrefix(list, "[") && strings.HasSuffix(list, "]") {
		list = list[1 : len(list)-1]
	}

	items := strings.Split(list, ",")
	for _, item := range items {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, "'\"")
		if item == value {
			return true
		}
	}
	return false
}

func (s *Scheduler) executeChain(chainName, triggeredBy string, input map[string]interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
	defer cancel()

	exec, err := s.executor.Execute(ctx, chainName, ExecuteOptions{
		Input:       input,
		TriggeredBy: triggeredBy,
		Async:       false,
	})

	if err != nil {
		log.Printf("[chains] Chain %s failed: %v", chainName, err)
		return
	}

	log.Printf("[chains] Chain %s completed with status: %s", chainName, exec.Status)
}

// RegisterChain adds a new chain to the scheduler
func (s *Scheduler) RegisterChain(chain *ChainDefinition) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch chain.Trigger.Type {
	case TriggerScheduled:
		if chain.Trigger.Cron != "" {
			return s.addCronJob(chain)
		}
	case TriggerEvent:
		if chain.Trigger.Event != "" {
			s.events[chain.Trigger.Event] = append(s.events[chain.Trigger.Event], chain.Name)
		}
	}

	return nil
}

// UnregisterChain removes a chain from the scheduler
func (s *Scheduler) UnregisterChain(chainName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove cron job
	if entryID, exists := s.jobs[chainName]; exists {
		s.cron.Remove(entryID)
		delete(s.jobs, chainName)
	}

	// Remove from event triggers
	for event, chains := range s.events {
		for i, name := range chains {
			if name == chainName {
				s.events[event] = append(chains[:i], chains[i+1:]...)
				break
			}
		}
	}

	return nil
}

// GetScheduledChains returns info about scheduled chains
func (s *Scheduler) GetScheduledChains() []ScheduledChainInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var info []ScheduledChainInfo
	for name, entryID := range s.jobs {
		entry := s.cron.Entry(entryID)
		chain, _ := s.registry.Get(name)

		info = append(info, ScheduledChainInfo{
			ChainName: name,
			Cron:      chain.Trigger.Cron,
			NextRun:   entry.Next,
			PrevRun:   entry.Prev,
		})
	}
	return info
}

// GetEventTriggers returns info about event triggers
func (s *Scheduler) GetEventTriggers() map[string][]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string][]string)
	for event, chains := range s.events {
		result[event] = append([]string{}, chains...)
	}
	return result
}

// ScheduledChainInfo provides information about a scheduled chain
type ScheduledChainInfo struct {
	ChainName string    `json:"chain_name"`
	Cron      string    `json:"cron"`
	NextRun   time.Time `json:"next_run"`
	PrevRun   time.Time `json:"prev_run"`
}

// RunNow manually triggers a scheduled chain immediately
func (s *Scheduler) RunNow(chainName string, input map[string]interface{}) (*ChainExecution, error) {
	return s.executor.Execute(context.Background(), chainName, ExecuteOptions{
		Input:       input,
		TriggeredBy: "manual",
		Async:       true,
	})
}

// IsRunning returns whether the scheduler is active
func (s *Scheduler) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}
