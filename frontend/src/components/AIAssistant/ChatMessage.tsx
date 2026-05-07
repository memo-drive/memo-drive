import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import "highlight.js/styles/github.css";
import { useTranslation } from "react-i18next";
import type { ChatMessage as ChatMessageType } from "../../stores/chatStore";
import { SourceReference } from "./SourceReference";
import styles from "./ChatMessage.module.css";

export function ChatMessage({ message }: { message: ChatMessageType }) {
  const { t } = useTranslation();
  const isAssistant = message.role === "assistant";

  return (
    <div className={`${styles.message} ${isAssistant ? styles.assistant : styles.user}`}>
      <div className={styles.bubble}>
        {isAssistant ? (
          <div className={styles.markdown}>
            {message.content ? (
              <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[rehypeHighlight]}
                components={{
                  a: ({ children, ...props }) => (
                    <a {...props} target="_blank" rel="noreferrer noopener">
                      {children}
                    </a>
                  ),
                  code: ({ className, children, ...props }) => (
                    <code
                      className={`${className ?? ""} ${className ? styles.codeBlock : styles.inlineCode}`}
                      {...props}
                    >
                      {children}
                    </code>
                  ),
                  pre: ({ children }) => <pre className={styles.pre}>{children}</pre>,
                  table: ({ children }) => <table className={styles.table}>{children}</table>,
                  th: ({ children }) => <th className={styles.th}>{children}</th>,
                  td: ({ children }) => <td className={styles.td}>{children}</td>,
                }}
              >
                {message.content}
              </ReactMarkdown>
            ) : message.streaming ? (
              <span className={styles.placeholder}>{t("ai.thinking")}</span>
            ) : null}
            {message.streaming ? <span className={styles.cursor}>▍</span> : null}
            {message.error ? <div className={styles.error}>{message.error}</div> : null}
          </div>
        ) : (
          <div className={styles.userText}>{message.content}</div>
        )}
      </div>
      {isAssistant && message.sources?.length ? (
        <SourceReference sources={message.sources} compact />
      ) : null}
    </div>
  );
}
