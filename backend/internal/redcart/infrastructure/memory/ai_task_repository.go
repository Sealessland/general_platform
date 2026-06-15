package memory

import (
	"fmt"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"time"
)

func (r *Repository) CreateAITask(task domain.AIGenerationTask) (domain.AIGenerationTask, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	task.ID = r.nextID(&r.nextAITaskID)
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now().UTC()
	}
	task.UpdatedAt = task.CreatedAt
	r.aiTasks[task.ID] = cloneAITask(task)
	return cloneAITask(task), nil
}

func (r *Repository) UpdateAITask(task domain.AIGenerationTask) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.aiTasks[task.ID]; !ok {
		return fmt.Errorf("ai task not found")
	}
	task.UpdatedAt = time.Now().UTC()
	r.aiTasks[task.ID] = cloneAITask(task)
	return nil
}

func (r *Repository) GetAITask(id int64) (domain.AIGenerationTask, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	task, ok := r.aiTasks[id]
	return cloneAITask(task), ok
}
