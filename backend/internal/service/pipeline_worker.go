package service

import (
	"context"
	"fmt"
	"log"

	"github.com/memodrive/backend/internal/model"
)

func (s *PipelineService) Requeue(ctx context.Context, taskID string, file *model.File) error {
	if err := s.queueTask(taskID, file); err != nil {
		s.failTask(ctx, taskID, file.ID, err)
		return err
	}
	return nil
}

func (s *PipelineService) Shutdown(ctx context.Context) error {
	if s == nil || s.runner == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		s.runner.Stop()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *PipelineService) queueTask(taskID string, file *model.File) error {
	return s.runner.Submit(func() {
		defer func() {
			if value := recover(); value != nil {
				err := fmt.Errorf("pipeline panic: %v", value)
				log.Printf("level=error component=pipeline event=panic task_id=%s file_id=%s err=%q", taskID, file.ID, err)
				s.failTask(context.Background(), taskID, file.ID, err)
			}
		}()
		s.run(context.Background(), taskID, file)
	})
}
