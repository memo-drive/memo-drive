import { useAIChat } from "../../hooks/useAIChat";
import { useAISearch } from "../../hooks/useAISearch";
import { useChatStore } from "../../stores/chatStore";
import { MobileAIView } from "./MobileAIView";
import { submitMobileAIQuery } from "./mobileAIQuery";

export function MobileAIPage() {
  const mode = useChatStore((state) => state.mode);
  const setMode = useChatStore((state) => state.setMode);
  const messages = useChatStore((state) => state.messages);
  const { sending, send, stop } = useAIChat();
  const { searchResults, searchQuery, loading, error, search } = useAISearch();
  const busy = sending || loading;

  return (
    <MobileAIView
      mode={mode}
      messages={messages}
      searchResults={searchResults}
      searchQuery={searchQuery}
      busy={busy}
      sending={sending}
      loading={loading}
      error={error}
      onModeChange={setMode}
      onSubmit={(query) =>
        submitMobileAIQuery({
          mode,
          query,
          busy,
          send,
          search,
        })
      }
      onStop={stop}
    />
  );
}
