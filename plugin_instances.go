package agent

import (
	"encoding/json"
	"fmt"
	"time"
)

// PluginInstance represents a user-configured plugin that is persisted in the database.
// On startup the engine restores all enabled instances from this store.
type PluginInstance struct {
	// ID is the unique instance identifier. For single-instance plugin types it
	// equals PluginName; for future multi-instance support it would include a
	// user/account suffix.
	ID         string         `json:"id"`
	PluginName string         `json:"plugin_name"` // registered factory name (e.g. "weixin")
	Category   string         `json:"category"`    // "dialog", "llm", "skill"
	Label      string         `json:"label"`       // human-readable display name
	Config     map[string]any `json:"config"`      // plugin-specific options
	Enabled    bool           `json:"enabled"`
	CreatedAt  int64          `json:"created_at"` // unix milliseconds
	UpdatedAt  int64          `json:"updated_at"` // unix milliseconds
}

// PluginInstanceStore persists PluginInstances in a KVStore.
type PluginInstanceStore struct {
	kv KVStore
}

// NewPluginInstanceStore creates a store backed by kv.
func NewPluginInstanceStore(kv KVStore) *PluginInstanceStore {
	return &PluginInstanceStore{kv: kv}
}

// Save persists inst. UpdatedAt is always refreshed; CreatedAt is set on first save.
func (s *PluginInstanceStore) Save(inst PluginInstance) error {
	now := time.Now().UnixMilli()
	inst.UpdatedAt = now
	if inst.CreatedAt == 0 {
		inst.CreatedAt = now
	}
	data, err := json.Marshal(inst)
	if err != nil {
		return fmt.Errorf("plugin_instances: marshal: %w", err)
	}
	return s.kv.Put([]byte(inst.ID), data)
}

// Get retrieves an instance by ID. Returns (nil, nil) when not found.
func (s *PluginInstanceStore) Get(id string) (*PluginInstance, error) {
	data, err := s.kv.Get([]byte(id))
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}
	var inst PluginInstance
	if err := json.Unmarshal(data, &inst); err != nil {
		return nil, fmt.Errorf("plugin_instances: unmarshal: %w", err)
	}
	return &inst, nil
}

// List returns all persisted instances.
func (s *PluginInstanceStore) List() ([]PluginInstance, error) {
	all, err := s.kv.List()
	if err != nil {
		return nil, err
	}
	result := make([]PluginInstance, 0, len(all))
	for _, data := range all {
		var inst PluginInstance
		if err := json.Unmarshal(data, &inst); err != nil {
			continue
		}
		result = append(result, inst)
	}
	return result, nil
}

// Delete permanently removes an instance by ID.
func (s *PluginInstanceStore) Delete(id string) error {
	return s.kv.Delete([]byte(id))
}
