package service

import (
	"context"

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/vectordb"
)

func (s *PipelineService) clearRetryArtifacts(ctx context.Context, file *model.File) error {
	if file.ChunkCount > 0 && s.vectorDB != nil {
		if err := s.vectorDB.Delete(ctx, vectordb.DefaultCollection, indexing.ChunkIDs(file.ID, file.ChunkCount)); err != nil {
			return err
		}
	}
	if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
		return err
	}
	if err := s.store.DeleteMetadataByFileID(ctx, file.ID); err != nil {
		return err
	}
	if err := s.store.UpdateFileChunkCount(ctx, file.ID, 0); err != nil {
		return err
	}
	if err := s.store.UpdateFileStatus(ctx, file.ID, model.FileStatusUploaded); err != nil {
		return err
	}
	file.ChunkCount = 0
	file.Status = model.FileStatusUploaded
	return nil
}
