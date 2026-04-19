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
