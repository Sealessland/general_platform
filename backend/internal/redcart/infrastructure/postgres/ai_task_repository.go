package postgres

import (
	"database/sql"
	"encoding/json"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
)

func (r *Repository) CreateAITask(task domain.AIGenerationTask) (domain.AIGenerationTask, error) {
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	outputJSON, err := json.Marshal(task.Output)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	err = r.db.QueryRow(
		`INSERT INTO ai_generation_tasks (user_id, merchant_id, task_type, input_json, output_json, status, error_message, created_at, updated_at)
		VALUES ($1,$2,$3,$4::jsonb,$5::jsonb,$6,$7,COALESCE($8, CURRENT_TIMESTAMP),COALESCE($9, CURRENT_TIMESTAMP))
		RETURNING id, created_at, updated_at`,
		nullInt64(task.UserID), nullInt64(task.MerchantID), task.TaskType, string(inputJSON), nullableJSON(outputJSON), task.Status, task.ErrorMessage, nullTime(task.CreatedAt), nullTime(task.UpdatedAt),
	).Scan(&task.ID, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	return task, nil
}

func (r *Repository) UpdateAITask(task domain.AIGenerationTask) error {
	inputJSON, err := json.Marshal(task.Input)
	if err != nil {
		return err
	}
	outputJSON, err := json.Marshal(task.Output)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(
		`UPDATE ai_generation_tasks SET user_id = $1, merchant_id = $2, task_type = $3, input_json = $4::jsonb, output_json = $5::jsonb, status = $6, error_message = $7 WHERE id = $8`,
		nullInt64(task.UserID), nullInt64(task.MerchantID), task.TaskType, string(inputJSON), nullableJSON(outputJSON), task.Status, task.ErrorMessage, task.ID,
	)
	return err
}

func (r *Repository) GetAITask(id int64) (domain.AIGenerationTask, bool) {
	row := r.db.QueryRow(`SELECT id, user_id, merchant_id, task_type, input_json, output_json, status, error_message, created_at, updated_at FROM ai_generation_tasks WHERE id = $1`, id)
	task, err := scanAITask(row)
	if err == sql.ErrNoRows {
		return domain.AIGenerationTask{}, false
	}
	return task, err == nil
}

type aiTaskScanner interface {
	Scan(dest ...any) error
}

func scanAITask(scanner aiTaskScanner) (domain.AIGenerationTask, error) {
	var task domain.AIGenerationTask
	var userID, merchantID sql.NullInt64
	var inputJSON []byte
	var outputJSON []byte
	err := scanner.Scan(&task.ID, &userID, &merchantID, &task.TaskType, &inputJSON, &outputJSON, &task.Status, &task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return domain.AIGenerationTask{}, err
	}
	task.UserID = userID.Int64
	task.MerchantID = merchantID.Int64
	_ = json.Unmarshal(inputJSON, &task.Input)
	if len(outputJSON) > 0 {
		_ = json.Unmarshal(outputJSON, &task.Output)
	}
	return task, nil
}
