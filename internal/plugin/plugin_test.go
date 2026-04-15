package plugin

import (
	"context"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

func TestExampleEnginePlugin_Lifecycle(t *testing.T) {
	plugin := NewExampleEnginePlugin()

	// Check metadata
	if plugin.Name() != "example-engine" {
		t.Fatalf("expected name 'example-engine', got %q", plugin.Name())
	}
	if plugin.Type() != PluginTypeEngine {
		t.Fatalf("expected type 'engine', got %q", plugin.Type())
	}

	// Should not be initialized or started yet
	if plugin.Health().Healthy {
		t.Fatal("expected unhealthy before start")
	}

	// Initialize
	if err := plugin.Initialize(map[string]interface{}{"key": "value"}); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Double init should fail
	if err := plugin.Initialize(nil); err == nil {
		t.Fatal("expected error on double initialize")
	}

	// Start
	ctx := context.Background()
	if err := plugin.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Should be healthy now
	status := plugin.Health()
	if !status.Healthy {
		t.Fatalf("expected healthy, got: %s", status.Message)
	}

	// Stop
	if err := plugin.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Should be unhealthy after stop
	if plugin.Health().Healthy {
		t.Fatal("expected unhealthy after stop")
	}
}

func TestExampleEnginePlugin_EngineCapabilities(t *testing.T) {
	plugin := NewExampleEnginePlugin()

	fields := plugin.SupportedFields()
	if len(fields) == 0 {
		t.Fatal("expected non-empty supported fields")
	}

	if plugin.MaxPageSize() <= 0 {
		t.Fatal("expected positive MaxPageSize")
	}

	rateLimit := plugin.RateLimit()
	if rateLimit.RequestsPerSecond <= 0 {
		t.Fatal("expected positive RequestsPerSecond")
	}
}

func TestExampleEnginePlugin_Translate(t *testing.T) {
	plugin := NewExampleEnginePlugin()

	// nil AST should error
	if _, err := plugin.Translate(nil); err == nil {
		t.Fatal("expected error for nil AST")
	}
}

func TestExampleEnginePlugin_Search_NotStarted(t *testing.T) {
	plugin := NewExampleEnginePlugin()

	// Search without start should fail
	if _, err := plugin.Search("test", 1, 10); err == nil {
		t.Fatal("expected error when searching before start")
	}
}

func TestPluginRegistry_RegisterAndGet(t *testing.T) {
	registry := NewPluginRegistry()
	plugin := NewExampleEnginePlugin()

	if err := registry.Register(plugin); err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	got, ok := registry.Get("example-engine")
	if !ok {
		t.Fatal("expected plugin to exist")
	}
	if got.Name() != "example-engine" {
		t.Fatalf("expected 'example-engine', got %q", got.Name())
	}
}

func TestPluginRegistry_DoubleRegister(t *testing.T) {
	registry := NewPluginRegistry()
	plugin := NewExampleEnginePlugin()

	if err := registry.Register(plugin); err != nil {
		t.Fatalf("first Register failed: %v", err)
	}

	if err := registry.Register(plugin); err == nil {
		t.Fatal("expected error on double register")
	}
}

func TestPluginRegistry_ListByType(t *testing.T) {
	registry := NewPluginRegistry()
	registry.Register(NewExampleEnginePlugin())

	engines := registry.GetEnginePlugins()
	if len(engines) != 1 {
		t.Fatalf("expected 1 engine plugin, got %d", len(engines))
	}

	processors := registry.GetProcessorPlugins()
	if len(processors) != 0 {
		t.Fatalf("expected 0 processor plugins, got %d", len(processors))
	}
}

func TestPluginManager_LoadAndStartPlugin(t *testing.T) {
	manager := NewPluginManager()
	defer manager.Shutdown()

	plugin := NewExampleEnginePlugin()
	if err := manager.LoadPlugin(plugin, map[string]interface{}{}); err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}

	if err := manager.StartPlugin("example-engine"); err != nil {
		t.Fatalf("StartPlugin failed: %v", err)
	}

	results := manager.HealthCheck()
	if !results["example-engine"].Healthy {
		t.Fatalf("expected healthy, got: %s", results["example-engine"].Message)
	}
}

func TestProcessorPipeline(t *testing.T) {
	// Create a simple processor plugin
	proc := &testProcessor{name: "test-processor", priority: 10}

	pipeline := NewProcessorPipeline([]ProcessorPlugin{proc})
	if len(pipeline.GetProcessors()) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(pipeline.GetProcessors()))
	}
}

// testProcessor is a minimal ProcessorPlugin for testing the pipeline.
type testProcessor struct {
	name     string
	priority int
}

func (t *testProcessor) Name() string                         { return t.name }
func (t *testProcessor) Version() string                      { return "0.1.0" }
func (t *testProcessor) Description() string                  { return "test" }
func (t *testProcessor) Author() string                       { return "test" }
func (t *testProcessor) Type() PluginType                     { return PluginTypeProcessor }
func (t *testProcessor) Initialize(map[string]interface{}) error { return nil }
func (t *testProcessor) Start(context.Context) error          { return nil }
func (t *testProcessor) Stop() error                          { return nil }
func (t *testProcessor) Health() HealthStatus {
	return HealthStatus{Healthy: true, Message: "ok"}
}
func (t *testProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	return assets, nil
}
func (t *testProcessor) Priority() int { return t.priority }
