export interface DriveFile {
  id: string;
  name: string;
  path: string;
  storage_path: string;
  size: number;
  mime_type: string;
  is_dir: boolean;
  parent_id?: string;
  status: "uploaded" | "processing" | "ready" | "failed" | string;
  chunk_count: number;
  created_at: string;
  updated_at: string;
  last_viewed_at?: string;
  deleted_at?: string;
  original_path?: string;
  original_name?: string;
  trash_root_id?: string;
  metadata?: FileMetadata;
}

export interface MarkdownContentResponse {
  file: DriveFile;
  content: string;
  updated_at: string;
}

export interface CreateMarkdownResponse {
  file: DriveFile;
}

export interface StorageUsage {
  used_bytes: number;
  total_bytes: number;
}

export interface FileMetadata {
  file_id: string;
  meta_json: string;
  thumbnail_path?: string;
  extracted_at: string;
}

export interface MediaMeta {
  width?: number;
  height?: number;
  duration?: number;
  taken_at?: string;
  latitude?: number;
  longitude?: number;
  camera?: string;
  codec?: string;
  bitrate?: number;
  format?: string;
}

export type UploadSessionStatus =
  | "uploading"
  | "merging"
  | "done"
  | "cancelled"
  | "expired"
  | "failed";

export interface UploadSession {
  id: string;
  file_name: string;
  file_size: number;
  chunk_size: number;
  uploaded_chunks: number[];
  dest_path: string;
  status: UploadSessionStatus;
  created_at: string;
  expires_at: string;
}

export interface UploadCompleteResponse {
  file: DriveFile;
  task_id: string;
}

export interface Task {
  id: string;
  file_id: string;
  type: string;
  status: string;
  progress: number;
  error?: string;
}

export interface SourceChunk {
  id: string;
  file_id: string;
  file_name: string;
  heading?: string;
  chunk_index: number;
  parent_chunk_id?: string;
  text: string;
  snippet: string;
  distance: number;
  score: number;
}

export interface AIChatMessage {
  role: "user" | "assistant" | "system" | string;
  content: string;
}

export interface RAGChatRequest {
  prompt: string;
  messages?: AIChatMessage[];
  file_ids?: string[];
  top_k?: number;
  conversation_id?: string;
}

export interface SearchRequest {
  query: string;
  file_ids?: string[];
  top_k?: number;
  conversation_id?: string;
}

export interface SearchIntent {
  text_query: string;
  mime_types?: string[];
  extensions?: string[];
  date_from?: string;
  date_to?: string;
  original: string;
}

export interface SearchResponse {
  conversation_id?: string;
  query: string;
  results: SourceChunk[] | null;
  intent?: SearchIntent;
}

export interface ConversationSummary {
  id: string;
  title: string;
  mode: "rag" | "search";
  file_ids?: string[];
  created_at: string;
  updated_at: string;
}

export interface ConversationMessage {
  id: string;
  conversation_id: string;
  role: "user" | "assistant";
  content: string;
  sources?: SourceChunk[];
  created_at: string;
}

export type FileMatchType = "name" | "meta" | "semantic" | "filter";

export interface FileSearchHit {
  file: DriveFile;
  match_types: FileMatchType[];
  snippet?: string;
  score: number;
}

export interface FileSearchResponse {
  query: string;
  total: number;
  hits: FileSearchHit[];
  semantic: boolean;
  intent?: SearchIntent;
}

export type FileQueryCategory = "photos" | "videos" | "documents" | "audio" | "all";

export type FileQueryDocumentSubtype =
  | "all"
  | "pdf"
  | "text"
  | "spreadsheet"
  | "presentation"
  | "txt"
  | "other";

export type FileQueryMediaFilter =
  | "all"
  | "lt_1m"
  | "1_10m"
  | "gt_10m";

export interface FileQueryRequest {
  category?: FileQueryCategory | string;
  query?: string;
  sort?: string;
  cursor?: string;
  limit?: number;
  media_filter?: FileQueryMediaFilter | string;
  document_subtype?: FileQueryDocumentSubtype | string;
}

export interface FileQueryResponse {
  items: DriveFile[];
  next_cursor: string;
  has_more: boolean;
}

export interface PhotoTimelineRequest {
  year: number;
  month: number;
  query?: string;
  sort?: string;
  cursor?: string;
  limit?: number;
}

export interface PhotoMonthIndexItem {
  year: number;
  month: number;
  count: number;
}

export interface PhotoMonthIndexResponse {
  months: PhotoMonthIndexItem[];
}

export interface BatchFileResult {
  total: number;
  succeeded: number;
  failed: number;
}

export type AISseEvent =
  | { type: "conversation"; id: string }
  | { type: "sources"; sources: SourceChunk[] }
  | { type: "delta"; delta: string }
  | { type: "error"; error: string }
  | { type: "done" };
