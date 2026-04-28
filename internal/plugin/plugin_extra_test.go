package plugin

import (
	"context"
	"testing"

	"github.com/unimap-icp-hunter/project/internal/model"
)

// ===== ExampleEnginePlugin metadata =====

func TestExampleEnginePlugin_Version(t *testing.T) {
	p := NewExampleEnginePlugin()
	if p.Version() != "0.1.0" {
		t.Errorf("version = %q, want 0.1.0", p.Version())
	}
}

func TestExampleEnginePlugin_Description(t *testing.T) {
	p := NewExampleEnginePlugin()
	if p.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestExampleEnginePlugin_Author(t *testing.T) {
	p := NewExampleEnginePlugin()
	if p.Author() != "unimap" {
		t.Errorf("author = %q, want unimap", p.Author())
	}
}

func TestExampleEnginePlugin_Normalize(t *testing.T) {
	p := NewExampleEnginePlugin()

	// nil result
	_, err := p.Normalize(nil)
	if err == nil {
		t.Fatal("expected error for nil raw result")
	}

	// valid result
	result := &model.EngineResult{EngineName: "test"}
	got, err := p.Normalize(result)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 assets, got %d", len(got))
	}
}

// ===== HookRegistry =====

func TestHookRegistry_RegisterAndTrigger(t *testing.T) {
	r := NewHookRegistry()
	var called string
	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error {
		called = name
		return nil
	})

	if r.CountHooks(HookBeforeLoad) != 1 {
		t.Errorf("expected 1 hook, got %d", r.CountHooks(HookBeforeLoad))
	}

	err := r.TriggerHook(HookBeforeLoad, "test-plugin", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called != "test-plugin" {
		t.Errorf("called = %q, want test-plugin", called)
	}
}

func TestHookRegistry_MultipleHooks(t *testing.T) {
	r := NewHookRegistry()
	callCount := 0

	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error {
		callCount++
		return nil
	})
	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error {
		callCount++
		return nil
	})

	err := r.TriggerHook(HookBeforeLoad, "test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestHookRegistry_HookError(t *testing.T) {
	r := NewHookRegistry()
	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error {
		return context.DeadlineExceeded
	})

	err := r.TriggerHook(HookBeforeLoad, "test-plugin", nil)
	if err == nil {
		t.Fatal("expected error from hook")
	}
}

func TestHookRegistry_TriggerNonExistent(t *testing.T) {
	r := NewHookRegistry()
	err := r.TriggerHook(HookAfterQuery, "test", nil)
	if err != nil {
		t.Fatalf("expected nil for non-existent hook type, got: %v", err)
	}
}

func TestHookRegistry_UnregisterHook(t *testing.T) {
	r := NewHookRegistry()
	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error {
		return nil
	})

	if r.CountHooks(HookBeforeLoad) != 1 {
		t.Fatalf("expected 1 hook before unregister")
	}

	r.UnregisterHook(HookBeforeLoad)
	if r.CountHooks(HookBeforeLoad) != 0 {
		t.Errorf("expected 0 hooks after unregister, got %d", r.CountHooks(HookBeforeLoad))
	}
}

func TestHookRegistry_ListHooks(t *testing.T) {
	r := NewHookRegistry()
	if len(r.ListHooks()) != 0 {
		t.Error("expected empty hooks list initially")
	}

	r.RegisterHook(HookBeforeLoad, func(name string, data map[string]interface{}) error { return nil })
	r.RegisterHook(HookAfterQuery, func(name string, data map[string]interface{}) error { return nil })

	types := r.ListHooks()
	if len(types) != 2 {
		t.Errorf("expected 2 hook types, got %d", len(types))
	}
}

func TestHookRegistry_CountHooks_Empty(t *testing.T) {
	r := NewHookRegistry()
	if r.CountHooks(HookBeforeLoad) != 0 {
		t.Errorf("expected 0 hooks, got %d", r.CountHooks(HookBeforeLoad))
	}
}

// ===== PluginRegistry: Unregister, ListByType, GetExporterPlugins, GetNotifierPlugins =====

func TestPluginRegistry_Unregister(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(NewExampleEnginePlugin())

	err := r.Unregister("example-engine")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, ok := r.Get("example-engine")
	if ok {
		t.Error("expected plugin to be unregistered")
	}
}

func TestPluginRegistry_Unregister_NonExistent(t *testing.T) {
	r := NewPluginRegistry()
	err := r.Unregister("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
}

func TestPluginRegistry_ListByType_Generic(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(NewExampleEnginePlugin())
	r.Register(&testProcessor{name: "proc1", priority: 1})

	engines := r.ListByType(PluginTypeEngine)
	if len(engines) != 1 {
		t.Errorf("expected 1 engine, got %d", len(engines))
	}

	processors := r.ListByType(PluginTypeProcessor)
	if len(processors) != 1 {
		t.Errorf("expected 1 processor, got %d", len(processors))
	}

	exporters := r.ListByType(PluginTypeExporter)
	if len(exporters) != 0 {
		t.Errorf("expected 0 exporters, got %d", len(exporters))
	}
}

func TestPluginRegistry_GetExporterPlugins(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(NewExampleEnginePlugin())
	exporters := r.GetExporterPlugins()
	if len(exporters) != 0 {
		t.Errorf("expected 0 exporter plugins, got %d", len(exporters))
	}
}

func TestPluginRegistry_GetNotifierPlugins(t *testing.T) {
	r := NewPluginRegistry()
	r.Register(NewExampleEnginePlugin())
	notifiers := r.GetNotifierPlugins()
	if len(notifiers) != 0 {
		t.Errorf("expected 0 notifier plugins, got %d", len(notifiers))
	}
}

func TestPluginRegistry_Register_EmptyName(t *testing.T) {
	r := NewPluginRegistry()
	p := &testProcessor{name: "", priority: 1}
	err := r.Register(p)
	if err == nil {
		t.Fatal("expected error for empty plugin name")
	}
}

// ===== PluginManager: UnloadPlugin, StartAll, StopAll, GetRegistry, GetHooks =====

func TestPluginManager_StopAll_NoPlugins(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	err := mgr.StopAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginManager_StartAll_NoPlugins(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	err := mgr.StartAll()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginManager_GetRegistry(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	reg := mgr.GetRegistry()
	if reg == nil {
		t.Fatal("expected non-nil registry")
	}
}

func TestPluginManager_GetHooks(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	hooks := mgr.GetHooks()
	if hooks == nil {
		t.Fatal("expected non-nil hooks")
	}
}

func TestPluginManager_StopPlugin_NonExistent(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	err := mgr.StopPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
}

func TestPluginManager_StartPlugin_NonExistent(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	err := mgr.StartPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
}

func TestPluginManager_UnloadPlugin(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()

	p := NewExampleEnginePlugin()
	if err := mgr.LoadPlugin(p, map[string]interface{}{}); err != nil {
		t.Fatalf("LoadPlugin failed: %v", err)
	}
	if err := mgr.StartPlugin("example-engine"); err != nil {
		t.Fatalf("StartPlugin failed: %v", err)
	}

	err := mgr.UnloadPlugin("example-engine")
	if err != nil {
		t.Fatalf("UnloadPlugin failed: %v", err)
	}

	_, ok := mgr.GetRegistry().Get("example-engine")
	if ok {
		t.Error("expected plugin to be unregistered after unload")
	}
}

func TestPluginManager_UnloadPlugin_NonExistent(t *testing.T) {
	mgr := NewPluginManager()
	defer mgr.Shutdown()
	err := mgr.UnloadPlugin("nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent plugin")
	}
}

// ===== ProcessorPipeline: Process, AddProcessor, RemoveProcessor =====

func TestProcessorPipeline_Process(t *testing.T) {
	proc := &testProcessor{name: "p1", priority: 1}
	pipeline := NewProcessorPipeline([]ProcessorPlugin{proc})

	ctx := context.Background()
	assets := []model.UnifiedAsset{{}}
	result, err := pipeline.Process(ctx, assets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 asset, got %d", len(result))
	}
}

func TestProcessorPipeline_Process_Error(t *testing.T) {
	proc := &failingProcessor{name: "fail"}
	pipeline := NewProcessorPipeline([]ProcessorPlugin{proc})

	ctx := context.Background()
	_, err := pipeline.Process(ctx, nil)
	if err == nil {
		t.Fatal("expected error from failing processor")
	}
}

func TestProcessorPipeline_AddProcessor(t *testing.T) {
	pipeline := NewProcessorPipeline(nil)
	if len(pipeline.GetProcessors()) != 0 {
		t.Fatalf("expected 0 processors initially")
	}

	p1 := &testProcessor{name: "low", priority: 10}
	p2 := &testProcessor{name: "high", priority: 1}
	pipeline.AddProcessor(p1)
	pipeline.AddProcessor(p2)

	if len(pipeline.GetProcessors()) != 2 {
		t.Fatalf("expected 2 processors, got %d", len(pipeline.GetProcessors()))
	}

	// Should be sorted by priority (high first, i.e. lower number first)
	if pipeline.GetProcessors()[0].Name() != "high" {
		t.Errorf("expected first processor to be 'high', got %q", pipeline.GetProcessors()[0].Name())
	}
}

func TestProcessorPipeline_RemoveProcessor(t *testing.T) {
	pipeline := NewProcessorPipeline(nil)
	pipeline.AddProcessor(&testProcessor{name: "p1", priority: 1})
	pipeline.AddProcessor(&testProcessor{name: "p2", priority: 2})

	pipeline.RemoveProcessor("p1")
	if len(pipeline.GetProcessors()) != 1 {
		t.Errorf("expected 1 processor after removal, got %d", len(pipeline.GetProcessors()))
	}
	if pipeline.GetProcessors()[0].Name() != "p2" {
		t.Errorf("expected remaining processor to be 'p2', got %q", pipeline.GetProcessors()[0].Name())
	}
}

func TestProcessorPipeline_RemoveProcessor_NonExistent(t *testing.T) {
	pipeline := NewProcessorPipeline(nil)
	pipeline.AddProcessor(&testProcessor{name: "p1", priority: 1})

	// Removing non-existent should not panic
	pipeline.RemoveProcessor("nonexistent")
	if len(pipeline.GetProcessors()) != 1 {
		t.Errorf("expected 1 processor after removing non-existent, got %d", len(pipeline.GetProcessors()))
	}
}

func TestProcessorPipeline_Process_Empty(t *testing.T) {
	pipeline := NewProcessorPipeline(nil)
	ctx := context.Background()
	result, err := pipeline.Process(ctx, []model.UnifiedAsset{{}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 asset, got %d", len(result))
	}
}

// ===== failingProcessor =====

type failingProcessor struct {
	name string
}

func (f *failingProcessor) Name() string                         { return f.name }
func (f *failingProcessor) Version() string                      { return "0.1.0" }
func (f *failingProcessor) Description() string                  { return "failing" }
func (f *failingProcessor) Author() string                       { return "test" }
func (f *failingProcessor) Type() PluginType                     { return PluginTypeProcessor }
func (f *failingProcessor) Initialize(map[string]interface{}) error { return nil }
func (f *failingProcessor) Start(context.Context) error          { return nil }
func (f *failingProcessor) Stop() error                          { return nil }
func (f *failingProcessor) Health() HealthStatus {
	return HealthStatus{Healthy: true, Message: "ok"}
}
func (f *failingProcessor) Process(ctx context.Context, assets []model.UnifiedAsset) ([]model.UnifiedAsset, error) {
	return nil, context.DeadlineExceeded
}
func (f *failingProcessor) Priority() int { return 1 }
