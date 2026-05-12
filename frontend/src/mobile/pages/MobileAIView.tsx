import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ChatMessage as ChatMessageBubble } from "../../components/AIAssistant/ChatMessage";
import type { AIMode, ChatMessage } from "../../stores/chatStore";
import type { SourceChunk } from "../../types";
import styles from "./MobileAIView.module.css";

interface MobileAIViewProps {
  mode: AIMode;
  messages: ChatMessage[];
  searchResults?: SourceChunk[] | null;
  searchQuery: string;
  busy: boolean;
  sending: boolean;
  loading: boolean;
  error: string;
  onModeChange: (mode: AIMode) => void;
  onSubmit: (query: string) => void | Promise<unknown>;
  onStop: () => void;
  onOpenSource?: (source: SourceChunk) => void | Promise<void>;
  backHref?: string;
}

export function MobileAIView({
  mode,
  messages,
  searchResults,
  searchQuery,
  busy,
  sending,
  loading,
  error,
  onModeChange,
  onSubmit,
  onStop,
  onOpenSource,
  backHref = "/m",
}: MobileAIViewProps) {
  const { t } = useTranslation();
  const [query, setQuery] = useState("");
  const placeholder = mode === "rag" ? t("ai.placeholderRag") : t("ai.placeholderSearch");

  async function submit(event: FormEvent) {
    event.preventDefault();
    const text = query.trim();
    if (!text || busy) return;
    setQuery("");
    await onSubmit(text);
  }

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <Link className={styles.backButton} to={backHref} aria-label={t("common.back")}>
          <span className="material-symbols-outlined" aria-hidden>
            arrow_back
          </span>
        </Link>
        <div className={styles.titleGroup}>
          <h1>{t("mobile.ai.title")}</h1>
        </div>
        <div className={styles.modeToggle} role="tablist" aria-label={t("ai.modeLabel")}>
          <button
            className={`${styles.modeButton} ${mode === "rag" ? styles.modeButtonActive : ""}`}
            type="button"
            role="tab"
            aria-selected={mode === "rag"}
            onClick={() => onModeChange("rag")}
          >
            {t("ai.modeRag")}
          </button>
          <button
            className={`${styles.modeButton} ${mode === "search" ? styles.modeButtonActive : ""}`}
            type="button"
            role="tab"
            aria-selected={mode === "search"}
            onClick={() => onModeChange("search")}
          >
            {t("ai.modeSearch")}
          </button>
        </div>
      </header>

      <main
        className={styles.results}
        role="log"
        aria-label={t("mobile.ai.conversation")}
        aria-live={busy ? "polite" : "off"}
        data-mobile-ai-scroll="content"
      >
        {mode === "rag" ? (
          messages.length > 0 ? (
            <section className={styles.messageList}>
              {messages.map((message) => (
                <ChatMessageBubble
                  key={message.id}
                  message={message}
                  onOpenSource={onOpenSource}
                />
              ))}
            </section>
          ) : (
            <EmptyState
              icon="auto_awesome"
              title={t("smartSearch.emptyRagTitle")}
              hint={t("ai.emptyRag")}
            />
          )
        ) : (
          <SearchResultPanel
            query={searchQuery}
            results={searchResults}
            loading={loading}
            error={error}
            onOpenSource={onOpenSource}
          />
        )}
      </main>

      <footer
        className={styles.inputBar}
        aria-label={t("mobile.ai.composer")}
        data-mobile-ai-composer="fixed"
      >
        <form className={styles.form} onSubmit={submit}>
          <textarea
            value={query}
            rows={1}
            placeholder={placeholder}
            disabled={busy}
            onChange={(event) => setQuery(event.target.value)}
          />
          {sending ? (
            <button className={styles.iconButton} type="button" onClick={onStop} aria-label={t("ai.stop")}>
              <span className="material-symbols-outlined" aria-hidden>
                stop
              </span>
            </button>
          ) : (
            <button className={styles.submitButton} type="submit" disabled={!query.trim() || busy}>
              {loading ? t("ai.searching") : t("ai.send")}
            </button>
          )}
        </form>
      </footer>
    </section>
  );
}

function SearchResultPanel({
  query,
  results,
  loading,
  error,
  onOpenSource,
}: {
  query: string;
  results?: SourceChunk[] | null;
  loading: boolean;
  error: string;
  onOpenSource?: (source: SourceChunk) => void | Promise<void>;
}) {
  const { t } = useTranslation();
  const safeResults = Array.isArray(results) ? results : [];

  if (!query) {
    return (
      <EmptyState
        icon="travel_explore"
        title={t("smartSearch.emptySearchTitle")}
        hint={t("ai.emptySearch")}
      />
    );
  }

  if (loading) {
    return <div className={styles.state}>{t("ai.searching")}</div>;
  }

  if (error) {
    return <div className={styles.error}>{error}</div>;
  }

  if (safeResults.length === 0) {
    return <EmptyState icon="search_off" title={t("ai.noResults")} hint={t("smartSearch.noResultsHint")} />;
  }

  return (
    <section className={styles.searchList}>
      <div className={styles.searchHeader}>
        <span>{t("ai.searchQuery", { query })}</span>
        <strong>{t("smartSearch.resultCount", { count: safeResults.length })}</strong>
      </div>
      {safeResults.map((source, index) => (
        <button
          key={`${source.id}-${index}`}
          className={styles.resultCard}
          type="button"
          aria-label={t("smartSearch.openSource")}
          onClick={() => onOpenSource?.(source)}
        >
          <div className={styles.resultTop}>
            <span className="material-symbols-outlined" aria-hidden>
              description
            </span>
            <div>
              <h2>{source.file_name || t("smartSearch.unknownFile")}</h2>
              <p>{source.heading || t("smartSearch.untitledSection")}</p>
            </div>
            <strong>{Math.round(source.score * 100)}%</strong>
          </div>
          <p>{source.snippet || source.text}</p>
        </button>
      ))}
    </section>
  );
}

function EmptyState({
  icon,
  title,
  hint,
}: {
  icon: string;
  title: string;
  hint: string;
}) {
  return (
    <section className={styles.empty}>
      <span className="material-symbols-outlined" aria-hidden>
        {icon}
      </span>
      <h2>{title}</h2>
      <p>{hint}</p>
    </section>
  );
}
