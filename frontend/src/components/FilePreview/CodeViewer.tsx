import { useEffect, useMemo, useState } from "react";
import hljs from "highlight.js";
import "highlight.js/styles/github.css";
import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import { getDownloadText } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { PreviewShell } from "./PreviewShell";
import { PreviewToolbar } from "./PreviewToolbar";
import { PreviewError, PreviewLoading, PreviewUnsupported } from "./PreviewState";
import { isMarkdownFile, languageForFile } from "./textTypes";
import styles from "./FilePreview.module.css";

const MAX_TEXT_BYTES = 1 * 1024 * 1024;
const MAX_RENDER_CHARS = 200_000;
const MAX_RENDER_LINES = 5_000;
const MARKDOWN_RENDER_BYTES = 200 * 1024;
const AUTO_DETECT_SUBSET = ["typescript", "javascript", "go", "python", "json"];

interface PreparedContent {
  text: string;
  lineCount: number;
  truncated: boolean;
}

function prepareContent(content: unknown): PreparedContent {
  const raw = typeof content === "string" ? content : "";
  let text = raw;
  let truncated = false;
  if (!raw) {
    return { text: "", lineCount: 0, truncated: false };
  }
  if (text.length > MAX_RENDER_CHARS) {
    text = text.slice(0, MAX_RENDER_CHARS);
    truncated = true;
  }

  const lines = text.split(/\r?\n/);
  if (lines.length > MAX_RENDER_LINES) {
    text = lines.slice(0, MAX_RENDER_LINES).join("\n");
    truncated = true;
  }

  if (truncated) {
    text += "\n\n... [文件已截断，剩余部分请下载查看]";
  }

  return {
    text,
    lineCount: lines.length > MAX_RENDER_LINES ? lines.length : text.split(/\r?\n/).length,
    truncated,
  };
}

function highlightContent(content: string, language?: string): string {
  if (language && language !== "plaintext" && hljs.getLanguage(language)) {
    return hljs.highlight(content, { language, ignoreIllegals: true }).value;
  }
  return hljs.highlightAuto(content, AUTO_DETECT_SUBSET).value;
}

export function CodeViewer({ file }: { file: DriveFile }) {
  const [content, setContent] = useState("");
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [reloadKey, setReloadKey] = useState(0);

  useEffect(() => {
    if (file.size > MAX_TEXT_BYTES) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    getDownloadText(file.id)
      .then((text) => {
        if (!cancelled) setContent(text);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : "文本加载失败");
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [file.id, file.size, reloadKey]);

  const prepared = useMemo(() => prepareContent(content), [content]);
  const language = languageForFile(file);
  const highlighted = useMemo(
    () => highlightContent(prepared.text, language),
    [language, prepared.text],
  );
  const lineNumbers = useMemo(
    () =>
      Array.from({ length: prepared.lineCount }, (_, index) => index + 1).join(
        "\n",
      ),
    [prepared.lineCount],
  );

  if (file.size > MAX_TEXT_BYTES) {
    return <PreviewUnsupported file={file} reason="文件过大，请下载查看" />;
  }

  if (loading) return <PreviewLoading label="加载文本..." />;
  if (error) {
    return (
      <PreviewError
        message="文本加载失败"
        onRetry={() => setReloadKey((value) => value + 1)}
      />
    );
  }

  const renderMarkdown = isMarkdownFile(file) && file.size < MARKDOWN_RENDER_BYTES;

  return (
    <PreviewShell
      className={styles.codeShell}
      mode="padded"
      toolbar={
        <PreviewToolbar>
          <span className={styles.codeToolbarLabel}>
            {renderMarkdown ? "Markdown 预览" : language || "自动识别"}
          </span>
          {prepared.truncated && (
            <span className={styles.pageIndicator}>已截断显示</span>
          )}
        </PreviewToolbar>
      }
    >
      {renderMarkdown ? (
        <article className={styles.markdownBody}>
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeHighlight]}
          >
            {prepared.text}
          </ReactMarkdown>
          {prepared.truncated && (
            <p className={styles.truncationNote}>
              文件内容较长，仅显示前面部分。
            </p>
          )}
        </article>
      ) : (
        <>
          <div className={styles.codeFrame}>
            <pre className={styles.lineNumbers} aria-hidden="true">
              {lineNumbers}
            </pre>
            <pre className={styles.codeBlock}>
              <code
                className={`hljs language-${language || "plaintext"}`}
                dangerouslySetInnerHTML={{ __html: highlighted }}
              />
            </pre>
          </div>
          {prepared.truncated && (
            <p className={styles.truncationNote}>
              文件内容较长，仅显示前面部分。
            </p>
          )}
        </>
      )}
    </PreviewShell>
  );
}
