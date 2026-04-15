package plugin

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// ExampleEnginePlugin is a minimal EnginePlugin implementation used for
// testing and demonstrating the plugin system.
type ExampleEnginePlugin struct {
	name        string
	version     string
	description string
	author      string
	started     atomic.Bool
	initialized atomic.Bool
	config      map[string]interface{}
}

// NewExampleEnginePlugin creates a new example engine plugin.
func NewExampleEnginePlugin() *ExampleEnginePlugin {
	return &ExampleEnginePlugin{
		name:        "example-engine",
		version:     "0.1.0",
		description: "A minimal example engine plugin for testing the plugin system",
		author:      "unimap",
	}
}

// Plugin interface methods

func (e *ExampleEnginePlugin) Name() string {
	return e.name
}

func (e *ExampleEnginePlugin) Version() string {
	return e.version
}

func (e *ExampleEnginePlugin) Description() string {
	return e.description
}

func (e *ExampleEnginePlugin) Author() string {
	return e.author
}

func (e *ExampleEnginePlugin) Type() PluginType {
	return PluginTypeEngine
}

func (e *ExampleEnginePlugin) Initialize(config map[string]interface{}) error {
	if e.initialized.Load() {
		return fmt.Errorf("plugin %s already initialized", e.name)
	}
	e.config = config
	e.initialized.Store(true)
	return nil
}

func (e *ExampleEnginePlugin) Start(ctx context.Context) error {
	if !e.initialized.Load() {
		return fmt.Errorf("plugin %s not initialized", e.name)
	}
	e.started.Store(true)
	return nil
}

func (e *ExampleEnginePlugin) Stop() error {
	e.started.Store(false)
	return nil
}

func (e *ExampleEnginePlugin) Health() HealthStatus {
	if !e.started.Load() {
		return HealthStatus{
			Healthy: false,
			Message: "plugin not started",
		}
	}
	return HealthStatus{
		Healthy: true,
		Message: "ok",
	}
}

// EnginePlugin Interface methods

func (e *ExampleEnginePlugin) Translate(ast *model.UQLAST) (string, error) {
	if ast == nil {
		return "", fmt.Errorf("ast is nil")
	}
	if ast.Root == nil {
		return "", fmt.Errorf("ast root is nil")
	}
	// Return a simple query string from the AST root
	return fmt.Sprintf("%+v", ast.Root), nil
}

func (e *ExampleEnginePlugin) Search(query string, page, pageSize int) (*model.EngineResult, error) {
	if !e.started.Load() {
		return nil, fmt.Errorf("plugin %s not started", e.name)
	}
	// Return an empty result for demonstration
	return &model.EngineResult{
		EngineName: e.name,
		Page:       page,
		Total:      0,
		RawData:    make([]interface{}, 0),
	}, nil
}

func (e *ExampleEnginePlugin) Normalize(raw *model.EngineResult) ([]model.UnifiedAsset, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw result is nil")
	}
	// Return empty slice for demonstration
	return make([]model.UnifiedAsset, 0), nil
}

func (e *ExampleEnginePlugin) SupportedFields() []string {
	return []string{"ip", "port", "protocol", "domain"}
}

func (e *ExampleEnginePlugin) MaxPageSize() int {
	return 100
}

func (e *ExampleEnginePlugin) RateLimit() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 1,
		RequestsPerMinute: 30,
	}
}
