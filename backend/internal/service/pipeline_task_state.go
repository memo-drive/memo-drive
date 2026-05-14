package service

import (
	"context"

	"github.com/memodrive/backend/internal/model"
)

func (s *PipelineService) markReady(ctx context.Context, taskID, fileID string) {
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusDone, pipelineProgressCompleted, nil)
	_ = s.store.UpdateFileStatus(ctx, fileID, model.FileStatusReady)
}

func (s *PipelineService) failTask(ctx context.Context, taskID, fileID string, err error) {
	errText := "pipeline failed"
	if err != nil {
		errText = err.Error()
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusFailed, pipelineProgressCompleted, &errText)
	_ = s.store.UpdateFileStatus(ctx, fileID, model.FileStatusFailed)
}
