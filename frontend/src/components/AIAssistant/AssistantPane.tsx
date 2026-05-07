import {
  useEffect,
  useMemo,
  useState,
  type FormEvent,
  type HTMLAttributes,
} from "react";
import { useTranslation } from "react-i18next";
import { useAIChat } from "../../hooks/useAIChat";
import { useAISearch } from "../../hooks/useAISearch";
import { useChatStore } from "../../stores/chatStore";
import { Button } from "../base";
import { ChatMessage } from "./ChatMessage";
import { ModeToggle } from "./ModeToggle";
import { SourceReference } from "./SourceReference";
import styles from "./AssistantPane.module.css";

interface AssistantPaneProps {
  floating?: boolean;
  onClose?: () => void;
  headerProps?: HTMLAttributes<HTMLDivElement>;
}

export function AssistantPane({
  floating = false,
  onClose,
  headerProps,
}: AssistantPaneProps) {
  const { t } = useTranslation();
  const [prompt, setPrompt] = useState("");
  const mode = useChatStore((state) => state.mode);
  const { messages, sending, send, stop } = useAIChat();
  const {
    searchResults,
    searchQuery,
    loading: searching,
    error: searchError,
    search,
  } = useAISearch();

  const SUGGESTIONS = useMemo(
    () => [
      t("ai.suggestion1"),
      t("ai.suggestion2"),
      t("ai.suggestion3"),
      t("ai.suggestion4"),
    ],
    [t],
  );

  useEffect(() => {
    if (mode === "search") {
      stop();
    }
  }, [mode, stop]);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    const text = prompt.trim();
    if (!text || sending || searching) return;
    setPrompt("");
    if (mode === "rag") {
      await send(text);
    } else {
      await search(text);
    }
  }

  function handleClose() {
    stop();
    onClose?.();
  }

  const busy = sending || searching;
  const placeholder =
    mode === "rag" ? t("ai.placeholderRag") : t("ai.placeholderSearch");

  return (
    <section className={`${styles.pane} ${floating ? styles.floating : ""}`}>
      <div className={styles.header} {...headerProps}>
        <div className={styles.headerLeft}>
          <span className="material-symbols-outlined">auto_awesome</span>
          <div>
            <p className={styles.eyebrow}>{t("ai.eyebrow")}</p>
            <h2>{t("ai.title")}</h2>
          </div>
        </div>
        {onClose ? (
          <button
            className={styles.closeBtn}
            onClick={handleClose}
            aria-label={t("common.close")}
          >
            <span className="material-symbols-outlined">close</span>
          </button>
        ) : null}
      </div>

      <div className={styles.body}>
        <ModeToggle />
        <div className={styles.scrollRegion}>
          {mode === "rag" ? (
            messages.length === 0 ? (
              <div className={styles.empty}>
                <p>{t("ai.emptyRag")}</p>
                <div className={styles.suggestions}>
                  {SUGGESTIONS.map((item) => (
                    <button
                      key={item}
                      type="button"
                      onClick={() => setPrompt(item)}
                    >
                      {item}
                    </button>
                  ))}
                </div>
              </div>
            ) : (
              messages.map((message) => (
                <ChatMessage key={message.id} message={message} />
              ))
            )
          ) : (
            <div className={styles.searchPane}>
              {searchQuery ? (
                <p className={styles.searchQuery}>
                  {t("ai.searchQuery", { query: searchQuery })}
                </p>
              ) : (
                <div className={styles.empty}>
                  <p>{t("ai.emptySearch")}</p>
                  <div className={styles.suggestions}>
                    {SUGGESTIONS.map((item) => (
                      <button
                        key={item}
                        type="button"
                        onClick={() => setPrompt(item)}
                      >
                        {item}
                      </button>
                    ))}
                  </div>
                </div>
              )}
              {searchError ? <p className={styles.errorText}>{searchError}</p> : null}
              <SourceReference
                sources={searchResults}
                title="搜索结果"
                loading={searching}
              />
              {!searching && searchQuery && searchResults.length === 0 && !searchError ? (
                <p className={styles.muted}>{t("ai.noResults")}</p>
              ) : null}
            </div>
          )}
        </div>
      </div>

      <form className={styles.inputForm} onSubmit={onSubmit}>
        <input
          className={styles.input}
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          placeholder={placeholder}
          disabled={busy}
        />
        <Button
          type={sending ? "button" : "submit"}
          disabled={searching}
          size="sm"
          onClick={sending ? stop : undefined}
        >
          {sending
            ? t("ai.stop")
            : searching
              ? t("ai.searching")
              : mode === "rag"
                ? t("ai.send")
                : "搜索"}
        </Button>
      </form>
    </section>
  );
}
