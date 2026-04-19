package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	cron "github.com/robfig/cron/v3"
)

// CronJob represents a persisted scheduled task.
type CronJob struct {
	ID          string    `json:"id"`
	Project     string    `json:"project"`
	SessionKey  string    `json:"session_key"`
	CronExpr    string    `json:"cron_expr"`
	Prompt      string    `json:"prompt"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	LastRun     time.Time `json:"last_run,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
}

// CronStore persists cron jobs.
type CronStore struct {
	kv KVStore
}

func NewCronStore(kv KVStore) (*CronStore, error) {
	return &CronStore{kv: kv}, nil
}

func (s *CronStore) Add(job *CronJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return s.kv.Put([]byte(job.ID), data)
}

func (s *CronStore) List() []*CronJob {
	items, err := s.kv.List()
	if err != nil {
		slog.Error("cron: failed to list jobs", "error", err)
		return nil
	}

	var jobs []*CronJob
	for _, v := range items {
		var j CronJob
		if err := json.Unmarshal(v, &j); err == nil {
			jobs = append(jobs, &j)
		}
	}
	// Sort by ID for consistency? BoltDB List (ForEach) is sorted by key anyway.
	return jobs
}

func (s *CronStore) Get(id string) *CronJob {
	data, err := s.kv.Get([]byte(id))
	if err != nil || data == nil {
		return nil
	}
	var j CronJob
	if err := json.Unmarshal(data, &j); err != nil {
		return nil
	}
	return &j
}

func (s *CronStore) MarkRun(id string, err error) {
	job := s.Get(id)
	if job == nil {
		return
	}
	job.LastRun = time.Now()
	if err != nil {
		job.LastError = err.Error()
	} else {
		job.LastError = ""
	}
	if err := s.Add(job); err != nil {
		slog.Warn("cron: failed to save job", "id", id, "error", err)
	}
}

// CronScheduler runs cron jobs.
type CronScheduler struct {
	store   *CronStore
	cron    *cron.Cron
	engine  *Engine
	mu      sync.RWMutex
	entries map[string]cron.EntryID
}

func NewCronScheduler(store *CronStore, engine *Engine) *CronScheduler {
	return &CronScheduler{
		store:   store,
		cron:    cron.New(),
		engine:  engine,
		entries: make(map[string]cron.EntryID),
	}
}

func (cs *CronScheduler) Start() error {
	jobs := cs.store.List()
	for _, job := range jobs {
		if job.Enabled {
			if err := cs.scheduleJob(job); err != nil {
				slog.Warn("cron: failed to schedule job", "id", job.ID, "error", err)
			}
		}
	}
	cs.cron.Start()
	return nil
}

func (cs *CronScheduler) scheduleJob(job *CronJob) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	jobID := job.ID
	entryID, err := cs.cron.AddFunc(job.CronExpr, func() {
		cs.executeJob(jobID)
	})
	if err != nil {
		return err
	}
	cs.entries[jobID] = entryID
	return nil
}

func (cs *CronScheduler) executeJob(jobID string) {
	// Simplified execution: find job in store
	job := cs.store.Get(jobID)

	if job == nil || !job.Enabled {
		return
	}

	slog.Info("cron: executing job", "id", jobID)
	// Integration with Engine.HandleCron will happen later
	err := cs.engine.HandleCron(job)
	cs.store.MarkRun(jobID, err)
}

func GenerateCronID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		slog.Warn("cron: failed to generate random id", "error", err)
	}
	return hex.EncodeToString(b)
}

func CronExprToHuman(expr string, _ Language) string {
	// Simplified human-readable conversion for now
	return expr
}
