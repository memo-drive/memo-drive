import {
  useEffect,
  useState,
  type FormEvent,
  type HTMLAttributes,
} from "react";
import { useAIChat } from "../../hooks/useAIChat";
import { useAISearch } from "../../hooks/useAISearch";
import { useChatStore } from "../../stores/chatStore";
import { Button } from "../base";
import { ChatMessage } from "./ChatMessage";
import { ModeToggle } from "./ModeToggle";
import { SourceReference } from "./SourceReference";
import styles from "./AssistantPane.module.css";

const SUGGESTIONS = ["总结这次搜索", "找出关键日期", "提取金额", "翻译结果"];

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
    mode === "rag" ? "问一个关于文件的问题..." : "搜索文件中的语义片段...";

  return (
    <section className={`${styles.pane} ${floating ? styles.floating : ""}`}>
      <div className={styles.header} {...headerProps}>
        <div className={styles.headerLeft}>
          <span className="material-symbols-outlined">auto_awesome</span>
          <div>
            <p className={styles.eyebrow}>AI Copilot</p>
            <h2>AI 助手</h2>
          </div>
        </div>
        {onClose ? (
          <button
            className={styles.closeBtn}
            onClick={handleClose}
            aria-label="关闭 AI 助手"
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
                <p>上传文档并完成索引后，就可以和文件直接对话。</p>
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
                <p className={styles.searchQuery}>搜索：{searchQuery}</p>
              ) : (
                <div className={styles.empty}>
                  <p>输入关键词或问题，我会直接返回最相关的文件片段。</p>
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
                <p className={styles.muted}>没有找到相关片段，换个说法试试看。</p>
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
          {sending ? "停止" : searching ? "搜索中" : mode === "rag" ? "发送" : "搜索"}
        </Button>
      </form>
    </section>
  );
}
