import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { getFile } from "../../api/fileApi";
import { message } from "../../components/base";
import { useAIChat } from "../../hooks/useAIChat";
import { useAISearch } from "../../hooks/useAISearch";
import { useChatStore } from "../../stores/chatStore";
import type { SourceChunk } from "../../types";
import { mobilePreviewHref } from "../utils/mobilePath";
import { MobileAIView } from "./MobileAIView";
import { submitMobileAIQuery } from "./mobileAIQuery";

export function MobileAIPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const mode = useChatStore((state) => state.mode);
  const setMode = useChatStore((state) => state.setMode);
  const messages = useChatStore((state) => state.messages);
  const { sending, send, stop } = useAIChat();
  const { searchResults, searchQuery, loading, error, search } = useAISearch();
  const busy = sending || loading;

  async function openSource(source: SourceChunk) {
    try {
      const file = await getFile(source.file_id);
      navigate(mobilePreviewHref(file.id, file.path || "/"));
    } catch {
      message.error(t("smartSearch.sourceMissing"));
    }
  }

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
      onOpenSource={openSource}
    />
  );
}
