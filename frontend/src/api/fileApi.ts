import { getToken, httpClient } from "./HttpClient";
import type {
	BatchFileResult,
	CreateMarkdownResponse,
	DriveFile,
	FileQueryRequest,
	FileQueryResponse,
	FileSearchResponse,
	MarkdownContentResponse,
	MediaMeta,
	PhotoMonthIndexResponse,
	PhotoTimelineRequest,
  StorageUsage,
} from "../types";

export async function listFiles(path: string, sort = "created_at") {
  return httpClient.get<{ files: DriveFile[] }>(
    `/files?path=${encodeURIComponent(path)}&sort=${encodeURIComponent(sort)}`,
  );
}

export async function getStorageUsage() {
  return httpClient.get<StorageUsage>("/storage/usage");
}

export async function createFolder(path: string, name: string) {
  return httpClient.post<DriveFile>("/folders", { path, name });
}

export async function createMarkdownFile(path: string, name: string) {
  return httpClient.post<CreateMarkdownResponse>("/files/markdown", { path, name });
}

export async function renameFile(id: string, name: string) {
  return httpClient.put<DriveFile>(`/files/${id}`, { name });
}

export async function moveFile(id: string, path: string) {
  return httpClient.put<DriveFile>(`/files/${id}`, { path });
}

export async function getFile(id: string) {
  return httpClient.get<DriveFile>(`/files/${id}`);
}

export async function getMarkdownContent(id: string) {
  return httpClient.get<MarkdownContentResponse>(`/files/${id}/content`);
}

export async function saveMarkdownContent(id: string, content: string, baseUpdatedAt: string) {
  return httpClient.put<MarkdownContentResponse>(`/files/${id}/content`, {
    content,
    base_updated_at: baseUpdatedAt,
  });
}

export async function markFileViewed(id: string) {
  return httpClient.post<DriveFile>(`/files/${id}/view`, {});
}

export async function getMetadata(id: string) {
  return httpClient.get<MediaMeta>(`/files/${id}/metadata`);
}

export async function searchFiles(request: {
  query: string;
  path?: string;
  mime?: string;
  semantic?: boolean;
  limit?: number;
}) {
  return httpClient.post<FileSearchResponse>("/files/search", request);
}

export async function queryFiles(request: FileQueryRequest) {
  return httpClient.post<FileQueryResponse>("/files/query", request);
}

export async function listRecentlyViewedFiles(limit?: number) {
  const suffix = limit === undefined ? "" : `?limit=${encodeURIComponent(String(limit))}`;
  return httpClient.get<{ files: DriveFile[] }>(`/files/recent${suffix}`);
}

export async function listPhotoMonths() {
  return httpClient.get<PhotoMonthIndexResponse>("/files/photos/months");
}

export async function queryPhotoTimeline(request: PhotoTimelineRequest) {
  return httpClient.post<FileQueryResponse>("/files/photos/timeline", request);
}

export async function getDownloadText(id: string): Promise<string> {
  const url = httpClient.assetUrl(`/files/${id}/download`);
  const headers = new Headers();
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(url, { headers });
  if (!response.ok) {
    const message = await response.text().catch(() => response.statusText);
    throw new Error(message || response.statusText);
  }
  return response.text();
}

export async function deleteFile(id: string) {
  return httpClient.delete<void>(`/files/${id}`);
}

export async function batchMoveFiles(fileIds: string[], path: string) {
  return httpClient.post<BatchFileResult>("/files/batch/move", {
    file_ids: fileIds,
    path,
  });
}

export async function batchDeleteFiles(fileIds: string[]) {
  return httpClient.post<BatchFileResult>("/files/batch/delete", {
    file_ids: fileIds,
  });
}

export async function listTrash() {
  return httpClient.get<{ files: DriveFile[] }>("/trash");
}

export async function restoreFile(id: string) {
  return httpClient.post<DriveFile>(`/trash/${id}/restore`, {});
}

export async function purgeFile(id: string) {
  return httpClient.delete<void>(`/trash/${id}`);
}

export async function emptyTrash() {
  return httpClient.delete<{ purged: number }>("/trash");
}

export function downloadUrl(id: string) {
  return httpClient.assetUrl(`/files/${id}/download`);
}

export function thumbnailUrl(id: string) {
  return httpClient.assetUrl(`/files/${id}/thumbnail`);
}
