import { useCallback, useEffect, useRef, useState } from "react";
import { streamRAGChat } from "../api/aiApi";
import { useChatStore } from "../stores/chatStore";
import type { AIChatMessage, SourceChunk } from "../types";

const HISTORY_LIMIT = 10;

export function useAIChat() {
  const [sending, setSending] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const assistantIdRef = useRef<string | null>(null);
  const { mode, messages, conversationId, setConversationId, addMessage, updateMessage } =
    useChatStore();

  const stop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    setSending(false);
    if (assistantIdRef.current) {
      updateMessage(assistantIdRef.current, { streaming: false });
      assistantIdRef.current = null;
    }
  }, [updateMessage]);

  useEffect(() => {
    return () => {
      stop();
    };
  }, [stop]);

  async function send(prompt: string) {
    const text = prompt.trim();
    if (!text || sending || mode !== "rag") return;

    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    const userId = crypto.randomUUID();
    const assistantId = crypto.randomUUID();
    assistantIdRef.current = assistantId;
    const history = buildHistory(useChatStore.getState().messages, text);

    addMessage({ id: userId, role: "user", content: text });
    addMessage({
      id: assistantId,
      role: "assistant",
      content: "",
      streaming: true,
    });
    setSending(true);

    let content = "";
    let completed = false;
    try {
      await streamRAGChat(
        { prompt: text, messages: history, conversation_id: conversationId },
        {
          onConversation: (id: string) => {
            setConversationId(id);
          },
          onSources: (sources: SourceChunk[]) => {
            updateMessage(assistantId, { sources });
          },
          onDelta: (delta: string) => {
            content += delta;
            updateMessage(assistantId, { content });
          },
          onError: (error: string) => {
            updateMessage(assistantId, { error, streaming: false });
            setSending(false);
          },
          onDone: () => {
            completed = true;
            updateMessage(assistantId, { streaming: false });
            setSending(false);
          },
        },
        controller.signal,
      );
      if (!completed && !controller.signal.aborted) {
        updateMessage(assistantId, { streaming: false });
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        updateMessage(assistantId, {
          error: err instanceof Error ? err.message : "ai.chatFailed",
          streaming: false,
        });
      }
    } finally {
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
      if (assistantIdRef.current === assistantId) {
        assistantIdRef.current = null;
      }
      setSending(false);
    }
  }

  return { messages, sending, send, stop };
}

function buildHistory(
  messages: { role: "user" | "assistant"; content: string }[],
  prompt: string,
): AIChatMessage[] {
  const history = messages
    .filter((message) => message.content.trim())
    .slice(-HISTORY_LIMIT)
    .map((message) => ({
      role: message.role,
      content: message.content,
    }));
  history.push({ role: "user", content: prompt });
  return history;
}
