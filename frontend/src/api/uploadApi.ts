import { httpClient } from "./HttpClient";
import type { UploadProgressUpdate } from "./HttpClient";
import type {
  FileConflictPolicy,
  DirectoryPrepareResponse,
  UploadCompleteResponse,
  UploadSession,
} from "../types";

export interface LocalDirectoryUploadEntry {
  clientId: string;
  relativePath: string;
  file: File;
}

export async function prepareDirectoryUpload(
  destPath: string,
  entries: LocalDirectoryUploadEntry[],
) {
  return httpClient.post<DirectoryPrepareResponse>("/upload/directory/prepare", {
    dest_path: destPath,
    entries: entries.map((entry) => ({
      client_id: entry.clientId,
      relative_path: entry.relativePath,
      file_size: entry.file.size,
    })),
  });
}

export async function initUpload(
  file: File,
  destPath: string,
  overwritePolicy: FileConflictPolicy = "reject",
) {
  return httpClient.post<UploadSession>("/upload/init", {
    file_name: file.name,
    file_size: file.size,
    dest_path: destPath,
    overwrite_policy: overwritePolicy,
  });
}

export async function uploadChunk(
  uploadId: string,
  index: number,
  chunk: Blob,
  signal?: AbortSignal,
  onProgress?: (progress: UploadProgressUpdate) => void,
) {
  return httpClient.patchBlob<{
    upload_id: string;
    uploaded_chunks: number[];
  }>(`/upload/${uploadId}?chunk=${index}`, chunk, signal, onProgress);
}

export async function completeUpload(uploadId: string) {
  return httpClient.post<UploadCompleteResponse>(
    `/upload/${uploadId}/complete`,
  );
}

export async function getUploadSession(uploadId: string) {
  return httpClient.get<UploadSession>(`/upload/${uploadId}`);
}

export async function listUploadSessions(limit = 100) {
  return httpClient.get<{ sessions: UploadSession[] }>(
    `/upload/sessions?limit=${limit}`,
  );
}

export async function cancelUpload(uploadId: string) {
  return httpClient.delete<void>(`/upload/${uploadId}`);
}

export async function deleteUploadSession(uploadId: string) {
  return httpClient.delete<void>(`/upload/sessions/${uploadId}`);
}

export async function clearUploadSessions() {
  return httpClient.delete<{ deleted: number }>("/upload/sessions");
}
