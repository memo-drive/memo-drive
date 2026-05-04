import { useState, type FormEvent } from "react";
import { ConversationDrawer } from "../../components/AIAssistant/ConversationDrawer";
import { ModeToggle } from "../../components/AIAssistant/ModeToggle";
import { Button } from "../../components/base";
import { useAIChat } from "../../hooks/useAIChat";
import { useAISearch } from "../../hooks/useAISearch";
import { useChatStore } from "../../stores/chatStore";
import { SmartSearchHero } from "./Hero";
import { SmartSearchResults } from "./Results";
import styles from "./index.module.css";

export function SmartSearchPage() {
  const [query, setQuery] = useState("");
  const [historyOpen, setHistoryOpen] = useState(false);
  const mode = useChatStore((state) => state.mode);
  const conversationId = useChatStore((state) => state.conversationId);
  const setConversationId = useChatStore((state) => state.setConversationId);
  const { sending, send } = useAIChat();
  const { loading, search } = useAISearch();
  const messages = useChatStore((state) => state.messages);
  const searchQuery = useChatStore((state) => state.searchQuery);

  const hasContent = mode === "rag" ? messages.length > 0 : !!searchQuery;
  const busy = sending || loading;

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const text = query.trim();
    if (!text || busy) return;
    if (mode === "search") {
      await search(text);
    } else {
      await send(text);
    }
    setQuery("");
  }

  const placeholder =
    mode === "rag" ? "问一个关于文件的问题..." : "搜索文件里的相关片段...";

  return (
    <div className={styles.page}>
      <div className={styles.toolbar}>
        <div>
          <p>AI Workspace</p>
          <h2>智能搜索</h2>
        </div>
        <div className={styles.toolbarActions}>
          <ModeToggle />
          {conversationId ? (
            <span className={styles.sessionBadge}>会话已保存</span>
          ) : null}
          <Button variant="ghost" onClick={() => setHistoryOpen(true)}>
            <span className="material-symbols-outlined">history</span>
            历史会话
          </Button>
          <Button variant="ghost" onClick={() => setConversationId(undefined)}>
            <span className="material-symbols-outlined">add</span>
            新对话
          </Button>
        </div>
      </div>

      <main className={styles.conversationArea}>
        {!hasContent && <SmartSearchHero onSuggestionClick={setQuery} />}
        <SmartSearchResults />
      </main>

      <div className={styles.inputBar}>
        <form className={styles.searchBox} onSubmit={onSubmit}>
          <span className="material-symbols-outlined">search</span>
          <input
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder={placeholder}
            disabled={busy}
          />
          <Button type="submit" loading={busy} disabled={!query.trim()}>
            {mode === "search" ? "搜索" : "Ask AI"}
          </Button>
        </form>
      </div>

      <ConversationDrawer
        open={historyOpen}
        onClose={() => setHistoryOpen(false)}
      />
    </div>
  );
}
