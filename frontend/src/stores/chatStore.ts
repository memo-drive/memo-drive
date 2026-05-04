import { create } from "zustand";
import { getConversation as fetchConversation } from "../api/aiApi";
import type { ConversationSummary, SearchIntent, SourceChunk } from "../types";

export type AIMode = "rag" | "search";

export interface ChatMessage {
  id: string;
  role: "user" | "assistant";
  content: string;
  sources?: SourceChunk[];
  error?: string;
  streaming?: boolean;
}

interface ChatState {
  mode: AIMode;
  messages: ChatMessage[];
  conversationId?: string;
  conversations: ConversationSummary[];
  searchResults: SourceChunk[];
  searchQuery: string;
  searchIntent?: SearchIntent;
  setMode: (mode: AIMode) => void;
  setConversationId: (id?: string) => void;
  setConversations: (list: ConversationSummary[]) => void;
  loadConversation: (id: string) => Promise<void>;
  addMessage: (message: ChatMessage) => void;
  updateMessage: (id: string, patch: Partial<ChatMessage> | string) => void;
  setSearchResults: (results: SourceChunk[]) => void;
  setSearchQuery: (query: string) => void;
  setSearchIntent: (intent?: SearchIntent) => void;
  clearSearch: () => void;
  clear: () => void;
}

const MAX_MESSAGES = 50;

function capMessages(messages: ChatMessage[]) {
  if (messages.length <= MAX_MESSAGES) return messages;
  return messages.slice(messages.length - MAX_MESSAGES);
}

export const useChatStore = create<ChatState>((set) => ({
  mode: "rag",
  messages: [],
  conversationId: undefined,
  conversations: [],
  searchResults: [],
  searchQuery: "",
  searchIntent: undefined,
  setMode: (mode) =>
    set(() => ({
      mode,
      searchResults: [],
      searchQuery: "",
      searchIntent: undefined,
    })),
  setConversationId: (conversationId) =>
    set(() =>
      conversationId
        ? { conversationId }
        : {
            conversationId: undefined,
            messages: [],
            searchResults: [],
            searchQuery: "",
            searchIntent: undefined,
          },
    ),
  setConversations: (conversations) => set({ conversations }),
  loadConversation: async (id) => {
    const response = await fetchConversation(id);
    const mode = response.conversation.mode === "search" ? "search" : "rag";
    set({
      conversationId: response.conversation.id,
      mode,
      messages: capMessages(
        response.messages
          .filter((message) => message.role === "user" || message.role === "assistant")
          .map((message) => ({
            id: message.id,
            role: message.role,
            content: message.content,
            sources: message.sources,
          })),
      ),
      searchResults:
        mode === "search"
          ? response.messages[response.messages.length - 1]?.sources ?? []
          : [],
      searchQuery:
        mode === "search"
          ? [...response.messages].reverse().find((message) => message.role === "user")
              ?.content ?? ""
          : "",
      searchIntent: undefined,
    });
  },
  addMessage: (message) =>
    set((state) => ({ messages: capMessages([...state.messages, message]) })),
  updateMessage: (id, patch) =>
    set((state) => ({
      messages: state.messages.map((message) =>
        message.id === id
          ? {
              ...message,
              ...(typeof patch === "string" ? { content: patch } : patch),
            }
          : message,
      ),
    })),
  setSearchResults: (searchResults) => set({ searchResults }),
  setSearchQuery: (searchQuery) => set({ searchQuery }),
  setSearchIntent: (searchIntent) => set({ searchIntent }),
  clearSearch: () => set({ searchResults: [], searchQuery: "", searchIntent: undefined }),
  clear: () => set({ messages: [], searchResults: [], searchQuery: "", searchIntent: undefined }),
}));
