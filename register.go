package agent

import (
	"fmt"
	"sort"
	"sync"
)

// ──────────────────────────────────────────────────────────────
// Platform (Dialog) Registry
// ──────────────────────────────────────────────────────────────

var (
	dialogFactories = make(map[string]DialogFactory)
	dialogMu        sync.RWMutex
)

// RegisterDialog registers a Dialog platform factory by name.
func RegisterDialog(name string, factory DialogFactory) {
	dialogMu.Lock()
	defer dialogMu.Unlock()
	dialogFactories[name] = factory
}

// ListDialogFactories returns the names of all currently registered Dialog platforms.
func ListDialogFactories() []string {
	dialogMu.RLock()
	defer dialogMu.RUnlock()
	list := make([]string, 0, len(dialogFactories))
	for name := range dialogFactories {
		list = append(list, name)
	}
	return list
}

// CreateDialog instantiates a Dialog platform by its registered name.
func CreateDialog(name string, opts map[string]any) (Dialog, error) {
	dialogMu.RLock()
	factory, ok := dialogFactories[name]
	dialogMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown dialog provider %q", name)
	}
	return factory(opts)
}

// ──────────────────────────────────────────────────────────────
// AI Model (LLM) Registry
// ──────────────────────────────────────────────────────────────

var (
	llmFactories = make(map[string]AgentFactory)
	llmMu        sync.RWMutex
)

// RegisterLLM registers an LLM provider factory by name.
func RegisterLLM(name string, factory AgentFactory) {
	llmMu.Lock()
	defer llmMu.Unlock()
	llmFactories[name] = factory
}

// ListLLMFactories returns the names of all currently registered LLM providers.
func ListLLMFactories() []string {
	llmMu.RLock()
	defer llmMu.RUnlock()
	list := make([]string, 0, len(llmFactories))
	for name := range llmFactories {
		list = append(list, name)
	}
	return list
}

// CreateLLM instantiates an LLM provider by its registered name.
func CreateLLM(name string, opts map[string]any) (LLM, error) {
	llmMu.RLock()
	factory, ok := llmFactories[name]
	llmMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown llm provider %q", name)
	}
	return factory(opts)
}

// ──────────────────────────────────────────────────────────────
// Pipe Registry
// ──────────────────────────────────────────────────────────────

type registeredPipeFactory struct {
	name     string
	priority int
	factory  PipeFactory
}

var (
	pipeFactories []registeredPipeFactory
	pipeMu        sync.RWMutex
)

// RegisterPipe registers a pipe factory with a specific priority.
// Higher priority pipes run later in the pipeline.
func RegisterPipe(name string, priority int, factory PipeFactory) {
	pipeMu.Lock()
	defer pipeMu.Unlock()
	pipeFactories = append(pipeFactories, registeredPipeFactory{name, priority, factory})
}

// CreatePipes creates and returns instances of all registered pipes, sorted by priority.
func CreatePipes(ctx PipeContext) []Pipe {
	pipeMu.RLock()
	defer pipeMu.RUnlock()

	// Create a shallow copy to safely sort without affecting the global registry
	tmp := make([]registeredPipeFactory, len(pipeFactories))
	copy(tmp, pipeFactories)

	sort.Slice(tmp, func(i, j int) bool {
		return tmp[i].priority < tmp[j].priority
	})

	instances := make([]Pipe, 0, len(tmp))
	for _, rf := range tmp {
		instances = append(instances, rf.factory(ctx))
	}
	return instances
}

// ──────────────────────────────────────────────────────────────
// Skill Manager Registry
// ──────────────────────────────────────────────────────────────

var (
	skillManagerFactories = make(map[string]SkillManagerFactory)
	skillManagerMu        sync.RWMutex
)

// RegisterSkillManager registers a SkillManager implementation factory by name.
func RegisterSkillManager(name string, factory SkillManagerFactory) {
	skillManagerMu.Lock()
	defer skillManagerMu.Unlock()
	skillManagerFactories[name] = factory
}

// ListSkillManagerFactories returns the names of all currently registered SkillManagers.
func ListSkillManagerFactories() []string {
	skillManagerMu.RLock()
	defer skillManagerMu.RUnlock()
	list := make([]string, 0, len(skillManagerFactories))
	for name := range skillManagerFactories {
		list = append(list, name)
	}
	return list
}

// CreateSkillManager instantiates a SkillManager by its registered name.
func CreateSkillManager(name string, opts map[string]any, storage KVStoreProvider) (SkillManager, error) {
	skillManagerMu.RLock()
	factory, ok := skillManagerFactories[name]
	skillManagerMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unknown skill manager provider %q", name)
	}
	if storage != nil {
		if opts == nil {
			opts = make(map[string]any)
		}
		opts["_storage"] = storage
	}
	return factory(opts)
}

// ──────────────────────────────────────────────────────────────
// Storage Backend Registry
// ──────────────────────────────────────────────────────────────

var (
	storageFactories = make(map[string]StorageFactory)
	storageMu        sync.RWMutex
)

// RegisterStorage registers a StorageBackend factory by name.
func RegisterStorage(name string, factory StorageFactory) {
	storageMu.Lock()
	defer storageMu.Unlock()
	storageFactories[name] = factory
}

// CreateStorage instantiates a StorageBackend by its registered name.
func CreateStorage(name string, opts map[string]any) (StorageBackend, error) {
	storageMu.RLock()
	factory, ok := storageFactories[name]
	storageMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown storage backend %q", name)
	}
	return factory(opts)
}

// ListStorageFactories returns the names of all registered storage backends.
func ListStorageFactories() []string {
	storageMu.RLock()
	defer storageMu.RUnlock()
	list := make([]string, 0, len(storageFactories))
	for name := range storageFactories {
		list = append(list, name)
	}
	return list
}

// ──────────────────────────────────────────────────────────────
// Trigger Registry
// ──────────────────────────────────────────────────────────────

var (
	triggerFactories = make(map[string]TriggerFactory)
	triggerMu        sync.RWMutex
)

// RegisterTrigger registers a Trigger factory by name.
func RegisterTrigger(name string, factory TriggerFactory) {
	triggerMu.Lock()
	defer triggerMu.Unlock()
	triggerFactories[name] = factory
}

// CreateTrigger instantiates a Trigger by its registered name.
func CreateTrigger(name string, opts map[string]any) (Trigger, error) {
	triggerMu.RLock()
	factory, ok := triggerFactories[name]
	triggerMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown trigger %q", name)
	}
	return factory(opts)
}

// ListTriggerFactories returns names of all registered triggers.
func ListTriggerFactories() []string {
	triggerMu.RLock()
	defer triggerMu.RUnlock()
	list := make([]string, 0, len(triggerFactories))
	for name := range triggerFactories {
		list = append(list, name)
	}
	return list
}

// ──────────────────────────────────────────────────────────────
// SecretProvider Registry
// ──────────────────────────────────────────────────────────────

var (
	secretFactories = make(map[string]SecretProviderFactory)
	secretMu        sync.RWMutex
)

// RegisterSecretProvider registers a SecretProvider factory by name.
func RegisterSecretProvider(name string, factory SecretProviderFactory) {
	secretMu.Lock()
	defer secretMu.Unlock()
	secretFactories[name] = factory
}

// CreateSecretProvider instantiates a SecretProvider by its registered name.
func CreateSecretProvider(name string, opts map[string]any) (SecretProvider, error) {
	secretMu.RLock()
	factory, ok := secretFactories[name]
	secretMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown secret provider %q", name)
	}
	return factory(opts)
}

// ──────────────────────────────────────────────────────────────
// EventBus Registry
// ──────────────────────────────────────────────────────────────

var (
	eventBusFactories = make(map[string]EventBusFactory)
	eventBusMu        sync.RWMutex
)

// RegisterEventBus registers an EventBus factory by name.
func RegisterEventBus(name string, factory EventBusFactory) {
	eventBusMu.Lock()
	defer eventBusMu.Unlock()
	eventBusFactories[name] = factory
}

// CreateEventBus instantiates an EventBus by its registered name.
func CreateEventBus(name string, opts map[string]any) (EventBus, error) {
	eventBusMu.RLock()
	factory, ok := eventBusFactories[name]
	eventBusMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown event bus %q", name)
	}
	return factory(opts)
}

// ──────────────────────────────────────────────────────────────
// PolicyEngine Registry
// ──────────────────────────────────────────────────────────────

var (
	policyFactories = make(map[string]PolicyEngineFactory)
	policyMu        sync.RWMutex
)

// RegisterPolicyEngine registers a PolicyEngine factory by name.
func RegisterPolicyEngine(name string, factory PolicyEngineFactory) {
	policyMu.Lock()
	defer policyMu.Unlock()
	policyFactories[name] = factory
}

// CreatePolicyEngine instantiates a PolicyEngine by its registered name.
func CreatePolicyEngine(name string, opts map[string]any) (PolicyEngine, error) {
	policyMu.RLock()
	factory, ok := policyFactories[name]
	policyMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown policy engine %q", name)
	}
	return factory(opts)
}
