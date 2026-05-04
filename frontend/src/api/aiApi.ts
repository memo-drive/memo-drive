import { httpClient } from "./HttpClient";
import type { SSEHandlers } from "./HttpClient";
import type {
  ConversationMessage,
  ConversationSummary,
  RAGChatRequest,
  SearchRequest,
  SearchResponse,
  SourceChunk,
} from "../types";

export interface RAGStreamHandlers {
  onDelta?: (delta: string) => void;
  onSources?: (sources: SourceChunk[]) => void;
  onConversation?: (id: string) => void;
  onError?: (error: string) => void;
  onDone?: () => void;
}

export function streamRAGChat(
  request: RAGChatRequest,
  handlers: RAGStreamHandlers,
  signal?: AbortSignal,
) {
  const sseHandlers: SSEHandlers = {
    onDelta: handlers.onDelta,
    onSources: (sources) => handlers.onSources?.(sources as SourceChunk[]),
    onConversation: handlers.onConversation,
    onError: handlers.onError,
    onDone: handlers.onDone,
  };
  return httpClient.streamSSE("/ai/chat", request, sseHandlers, signal);
}

export function semanticSearch(request: SearchRequest) {
  return httpClient.post<SearchResponse>("/ai/search", request);
}

export function listConversations(limit = 50, offset = 0) {
  return httpClient.get<{ conversations: ConversationSummary[] }>(
    `/conversations?limit=${limit}&offset=${offset}`,
  );
}

export function getConversation(id: string) {
  return httpClient.get<{
    conversation: ConversationSummary;
    messages: ConversationMessage[];
  }>(`/conversations/${id}`);
}

export function renameConversation(id: string, title: string) {
  return httpClient.patch<void>(`/conversations/${id}`, { title });
}

export function deleteConversation(id: string) {
  return httpClient.delete<void>(`/conversations/${id}`);
}

export async function streamChat(
  prompt: string,
  onDelta: (delta: string) => void,
) {
  return httpClient.postSSE("/ai/chat", { prompt }, onDelta);
}
