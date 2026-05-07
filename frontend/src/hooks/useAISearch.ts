import { useState } from "react";
import { semanticSearch } from "../api/aiApi";
import { useChatStore } from "../stores/chatStore";

export function useAISearch() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const {
    mode,
    searchResults,
    searchQuery,
    conversationId,
    setConversationId,
    setSearchQuery,
    setSearchResults,
    setSearchIntent,
  } = useChatStore();

  async function search(query: string) {
    const text = query.trim();
    if (!text || loading || mode !== "search") return;
    setLoading(true);
    setError("");
    setSearchQuery(text);
    try {
      const response = await semanticSearch({
        query: text,
        conversation_id: conversationId,
      });
      if (response.conversation_id) {
        setConversationId(response.conversation_id);
      }
      setSearchQuery(response.query);
      setSearchResults(response.results);
      setSearchIntent(response.intent);
    } catch (err) {
      const message = err instanceof Error ? err.message : "ai.semanticSearchFailed";
      setError(message);
      setSearchResults([]);
      setSearchIntent(undefined);
    } finally {
      setLoading(false);
    }
  }

  return { searchResults, searchQuery, loading, error, search };
}
