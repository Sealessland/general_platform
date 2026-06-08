package application_test

import (
	"context"
	"errors"
	"testing"

	backendai "github.com/example/redcart-copilot/backend/internal/ai"
	application "github.com/example/redcart-copilot/backend/internal/redcart/application"
	"github.com/example/redcart-copilot/backend/internal/redcart/domain"
	"github.com/example/redcart-copilot/backend/internal/redcart/infrastructure/memory"
)

func TestBusinessReviewGenerationAndTaskRead(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	consumer := application.Actor{UserID: 1, Role: domain.RoleConsumer}
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	if _, err := service.GenerateBusinessReview(context.Background(), consumer, application.BusinessReviewInput{WindowDays: 7}); !isAppError(err, application.ErrorForbidden) {
		t.Fatalf("expected consumer business review forbidden, got %v", err)
	}
	task, err := service.GenerateBusinessReview(context.Background(), merchant, application.BusinessReviewInput{WindowDays: 7})
	if err != nil {
		t.Fatalf("generate business review: %v", err)
	}
	if task.Status != domain.AITaskStatusCompleted || task.Output["diagnosis"] == nil {
		t.Fatalf("expected completed business review, got %+v", task)
	}
	fetched, err := service.GetAITask(context.Background(), merchant, task.ID)
	if err != nil {
		t.Fatalf("get ai task: %v", err)
	}
	if fetched.ID != task.ID || fetched.TaskType != domain.TaskTypeBusinessReview {
		t.Fatalf("expected fetched business review task, got %+v", fetched)
	}
}

func TestSellingPointGenerationPersistsReadableCompletedTask(t *testing.T) {
	t.Parallel()

	service := application.NewService(memory.NewRepository(), backendai.MockProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	task, err := service.GenerateSellingPoints(context.Background(), merchant, application.SellingPointInput{
		ProductName: "Travel Makeup Organizer",
		Attributes:  []string{"portable", "washable"},
		TargetUsers: "dorm users",
		PriceCent:   8900,
		Reviews:     []string{"fits my tiny desk"},
	})
	if err != nil {
		t.Fatalf("generate selling points: %v", err)
	}
	if task.Status != domain.AITaskStatusCompleted || task.TaskType != domain.TaskTypeSellingPoints {
		t.Fatalf("expected completed selling point task, got %+v", task)
	}
	if task.Input["product_name"] != "Travel Makeup Organizer" || task.Input["target_users"] != "dorm users" {
		t.Fatalf("expected persisted selling point input, got %+v", task.Input)
	}
	points, ok := task.Output["core_points"].([]string)
	if !ok || len(points) == 0 {
		t.Fatalf("expected core points output, got %+v", task.Output)
	}

	fetched, err := service.GetAITask(context.Background(), merchant, task.ID)
	if err != nil {
		t.Fatalf("get selling point task: %v", err)
	}
	titleSuggest, ok := fetched.Output["detail_title_suggest"].(string)
	if fetched.ID != task.ID || !ok || titleSuggest == "" {
		t.Fatalf("expected readable persisted task output, got %+v", fetched)
	}
}

func TestAIGenerationFailurePersistsFailedTask(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, failingAIProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	_, err := service.GenerateSellingPoints(context.Background(), merchant, application.SellingPointInput{
		ProductName: "Travel Bag",
		TargetUsers: "commuters",
	})
	if err == nil {
		t.Fatal("expected provider error")
	}

	tasks := []domain.AIGenerationTask{}
	for id := int64(1); id < 20; id++ {
		if task, ok := repo.GetAITask(id); ok {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed task, got %+v", tasks)
	}
	if tasks[0].Status != domain.AITaskStatusFailed || tasks[0].ErrorMessage == "" {
		t.Fatalf("expected failed task with error, got %+v", tasks[0])
	}
}

func TestBusinessReviewFailurePersistsFailedTask(t *testing.T) {
	t.Parallel()

	repo := memory.NewRepository()
	service := application.NewService(repo, failingAIProvider{})
	merchant := application.Actor{UserID: 2, Role: domain.RoleMerchant, MerchantID: 1}

	_, err := service.GenerateBusinessReview(context.Background(), merchant, application.BusinessReviewInput{WindowDays: 14})
	if err == nil {
		t.Fatal("expected provider error")
	}

	tasks := []domain.AIGenerationTask{}
	for id := int64(1); id < 20; id++ {
		if task, ok := repo.GetAITask(id); ok {
			tasks = append(tasks, task)
		}
	}
	if len(tasks) != 1 {
		t.Fatalf("expected one failed task, got %+v", tasks)
	}
	if tasks[0].TaskType != domain.TaskTypeBusinessReview || tasks[0].Status != domain.AITaskStatusFailed || tasks[0].ErrorMessage == "" {
		t.Fatalf("expected failed business review task with error, got %+v", tasks[0])
	}
	if tasks[0].Input["window_days"] != 14 {
		t.Fatalf("expected persisted business review input, got %+v", tasks[0].Input)
	}
}

type failingAIProvider struct{}

func (failingAIProvider) GenerateSellingPoints(context.Context, backendai.SellingPointRequest) (*backendai.SellingPointResult, error) {
	return nil, errors.New("provider unavailable")
}

func (failingAIProvider) GenerateBusinessReview(context.Context, backendai.BusinessReviewRequest) (*backendai.BusinessReviewResult, error) {
	return nil, errors.New("provider unavailable")
}
