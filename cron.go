package agent

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	path string
	mu   sync.Mutex
	jobs []*CronJob
}

func NewCronStore(dataDir string) (*CronStore, error) {
	dir := filepath.Join(dataDir, "crons")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "jobs.json")
	s := &CronStore{path: path}
	s.load()
	return s, nil
}

func (s *CronStore) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &s.jobs); err != nil {
		slog.Warn("cron: failed to unmarshal jobs", "error", err)
	}
}

func (s *CronStore) save() error {
	data, err := json.MarshalIndent(s.jobs, "", "  ")
	if err != nil {
		return fmt.Errorf("cron: marshal jobs: %w", err)
	}
	return AtomicWriteFile(s.path, data, 0o644)
}

func (s *CronStore) Add(job *CronJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs = append(s.jobs, job)
	return s.save()
}

func (s *CronStore) List() []*CronJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*CronJob, len(s.jobs))
	copy(out, s.jobs)
	return out
}

func (s *CronStore) MarkRun(id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			j.LastRun = time.Now()
			if err != nil {
				j.LastError = err.Error()
			} else {
				j.LastError = ""
			}
			if err := s.save(); err != nil {
				slog.Warn("cron: failed to save job", "id", id, "error", err)
			}
			return
		}
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
	cs.mu.Lock()
	// Simplified execution: find job in store
	var job *CronJob
	for _, j := range cs.store.jobs {
		if j.ID == jobID {
			job = j
			break
		}
	}
	cs.mu.Unlock()

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
