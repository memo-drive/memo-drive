import { httpClient } from "./HttpClient";
import type {
  PipelineTaskListPage,
  PipelineTaskRetryResponse,
  PipelineTaskStatus,
} from "../types";

export interface PipelineTaskListOptions {
  status?: PipelineTaskStatus;
  fileID?: string;
  cursor?: string;
  limit?: number;
}

export function listPipelineTasks(
  options: PipelineTaskListOptions = {},
  request: { background?: boolean } = {},
) {
  const query: string[] = [];
  if (options.status) query.push(`status=${encodeURIComponent(options.status)}`);
  if (options.fileID) query.push(`file_id=${encodeURIComponent(options.fileID)}`);
  if (options.cursor) query.push(`cursor=${encodeURIComponent(options.cursor)}`);
  if (options.limit !== undefined) query.push(`limit=${encodeURIComponent(String(options.limit))}`);
  const suffix = query.length > 0 ? `?${query.join("&")}` : "";
  const init = request.background
    ? ({ priority: "low" } as RequestInit & { priority: "low" })
    : undefined;
  return httpClient.get<PipelineTaskListPage>(`/tasks${suffix}`, init);
}

export function retryPipelineTask(taskID: string) {
  return httpClient.post<PipelineTaskRetryResponse>(
    `/tasks/${encodeURIComponent(taskID)}/retry`,
    {},
  );
}
