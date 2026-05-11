import type { AIMode } from "../../stores/chatStore";

interface SubmitMobileAIQueryOptions {
  mode: AIMode;
  query: string;
  busy: boolean;
  send: (query: string) => Promise<void> | void;
  search: (query: string) => Promise<void> | void;
}

export async function submitMobileAIQuery({
  mode,
  query,
  busy,
  send,
  search,
}: SubmitMobileAIQueryOptions) {
  const text = query.trim();
  if (!text || busy) return false;

  if (mode === "search") {
    await search(text);
  } else {
    await send(text);
  }

  return true;
}
