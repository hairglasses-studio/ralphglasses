package chains

import (
	"log"
	"os"
	"path/filepath"
	"sync"

	mcptools "github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// Engine holds all chain engine components
type Engine struct {
	Registry      *Registry
	Executor      *Executor
	Scheduler     *Scheduler
	Emitter       *EventEmitter
	Notifications *NotificationManager
	Templates     *TemplateRegistry
	StateStore    StateStore
	Metrics       *MetricsCollector

	mu      sync.Mutex
	running bool
}

// EngineConfig configures the chain engine
type EngineConfig struct {
	// DataDir is the directory for chain data (SQLite, etc.)
	DataDir string

	// ChainsDir is the directory containing chain YAML files
	ChainsDir string

	// ToolRegistry is the MCP tool registry for executing tools
	ToolRegistry *mcptools.ToolRegistry

	// SlackChannel for notifications (optional)
	SlackChannel string

	// SlackClient for sending notifications (optional)
	SlackClient SlackPoster

	// UseInMemoryState uses in-memory state instead of SQLite
	UseInMemoryState bool
}

var (
	globalEngine     *Engine
	globalEngineOnce sync.Once
)

// GetEngine returns the global chain engine instance
func GetEngine() *Engine {
	return globalEngine
}

// InitEngine initializes the global chain engine
func InitEngine(config EngineConfig) (*Engine, error) {
	var err error
	globalEngineOnce.Do(func() {
		globalEngine, err = NewEngine(config)
	})
	return globalEngine, err
}

// NewEngine creates a new chain engine with all components wired together
func NewEngine(config EngineConfig) (*Engine, error) {
	engine := &Engine{}

	// Initialize state store
	if config.UseInMemoryState {
		engine.StateStore = NewMemoryStateStore()
	} else {
		dbPath := filepath.Join(config.DataDir, "chains.db")
		if config.DataDir != "" {
			var err error
			engine.StateStore, err = NewSQLiteStateStore(dbPath)
			if err != nil {
				log.Printf("[chains] Failed to create SQLite store, using memory: %v", err)
				engine.StateStore = NewMemoryStateStore()
			}
		} else {
			engine.StateStore = NewMemoryStateStore()
		}
	}

	// Initialize registry
	engine.Registry = NewRegistry()

	// Register built-in chains
	if err := RegisterBuiltInChains(engine.Registry); err != nil {
		log.Printf("[chains] Failed to register built-in chains: %v", err)
	}

	// Load chains from directory if specified
	if config.ChainsDir != "" {
		if err := engine.Registry.LoadFromDirectory(config.ChainsDir); err != nil {
			log.Printf("[chains] Failed to load chains from %s: %v", config.ChainsDir, err)
		}
	}

	// Initialize template registry
	engine.Templates = NewTemplateRegistry()
	LoadBuiltInTemplates(engine.Templates)

	// Initialize tool invoker
	var invoker ToolInvoker
	if config.ToolRegistry != nil {
		invoker = NewMCPToolInvoker(config.ToolRegistry)
	}

	// Initialize metrics collector
	engine.Metrics = NewMetricsCollector()

	// Initialize executor
	engine.Executor = NewExecutor(engine.Registry, invoker, engine.StateStore)
	engine.Executor.SetMetricsCollector(engine.Metrics)

	// Initialize scheduler
	engine.Scheduler = NewScheduler(engine.Executor, engine.Registry)

	// Initialize event emitter
	engine.Emitter = NewEventEmitter(engine.Scheduler)

	// Initialize notification manager
	engine.Notifications = NewNotificationManager()
	engine.Notifications.RegisterHandler(NewLogNotificationHandler())

	if config.SlackClient != nil && config.SlackChannel != "" {
		engine.Notifications.RegisterHandler(
			NewSlackNotificationHandler(config.SlackClient, config.SlackChannel),
		)
	}

	// Wire notifications into executor
	engine.Executor.SetNotificationManager(engine.Notifications)

	log.Printf("[chains] Engine initialized with %d chains, %d templates",
		len(engine.Registry.List()), len(engine.Templates.List()))

	return engine, nil
}

// Start starts the chain engine (scheduler, event bridge)
func (e *Engine) Start() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return nil
	}

	// Start scheduler
	if err := e.Scheduler.Start(); err != nil {
		return err
	}

	e.running = true
	log.Printf("[chains] Engine started")
	return nil
}

// Stop stops the chain engine
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}

	e.Scheduler.Stop()

	if e.StateStore != nil {
		e.StateStore.Close()
	}

	e.running = false
	log.Printf("[chains] Engine stopped")
}

// IsRunning returns whether the engine is running
func (e *Engine) IsRunning() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// DefaultDataDir returns the default data directory for chains
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".webb/chains"
	}
	return filepath.Join(home, ".webb", "chains")
}

// DefaultChainsDir returns the default chains directory
func DefaultChainsDir() string {
	// Check for chains in working directory first
	if _, err := os.Stat("chains"); err == nil {
		return "chains"
	}

	// Check home directory
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	chainsDir := filepath.Join(home, ".webb", "chains")
	if _, err := os.Stat(chainsDir); err == nil {
		return chainsDir
	}

	return ""
}
