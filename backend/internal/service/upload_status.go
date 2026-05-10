package service

import "github.com/memodrive/backend/internal/model"

func uploadStatusCanExpire(status string) bool {
	return status == model.UploadStatusUploading
}

func canReceiveUploadChunk(status string) bool {
	return status == model.UploadStatusUploading
}

func canCompleteUploadStatus(status string) bool {
	return status == model.UploadStatusUploading
}

func canCancelUploadStatus(status string) bool {
	return status == model.UploadStatusUploading
}

func canDeleteUploadStatus(status string) bool {
	return status != model.UploadStatusMerging
}
