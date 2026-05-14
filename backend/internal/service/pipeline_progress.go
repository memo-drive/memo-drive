package service

// Pipeline progress milestones, reported as percentage via the task API.
const (
	pipelineProgressQueued    = 0   // task has been created but not yet picked up
	pipelineProgressStarted   = 15  // worker has started processing
	pipelineProgressParsed    = 30  // document/media text extraction complete
	pipelineProgressSplit     = 45  // hierarchical chunking complete
	pipelineProgressEmbedded  = 75  // vector embeddings generated
	pipelineProgressUpserted  = 90  // vectors upserted to ChromaDB
	pipelineProgressCompleted = 100 // indexing fully complete
)
