package agent

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCronStore(t *testing.T) {
	tmpDir := t.TempDir()
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	store, err := NewCronStore(tmpDir)
	assert.NoError(t, err)

	job := &CronJob{
		ID:        "job1",
		CronExpr:  "*/1 * * * *",
		Prompt:    "hello",
		Enabled:   true,
		CreatedAt: time.Now(),
	}

	err = store.Add(job)
	assert.NoError(t, err)

	jobs := store.List()
	assert.Len(t, jobs, 1)
	assert.Equal(t, "job1", jobs[0].ID)

	store.MarkRun("job1", nil)
	jobs = store.List()
	assert.False(t, jobs[0].LastRun.IsZero())
}

func TestGenerateCronID(t *testing.T) {
	id1 := GenerateCronID()
	id2 := GenerateCronID()
	assert.NotEmpty(t, id1)
	assert.NotEqual(t, id1, id2)
}
