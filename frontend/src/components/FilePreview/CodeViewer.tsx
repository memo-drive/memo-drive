import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import hljs from "highlight.js";
import "highlight.js/styles/github.css";
import ReactMarkdown from "react-markdown";
import rehypeHighlight from "rehype-highlight";
import remarkGfm from "remark-gfm";
import { getDownloadText } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { PreviewShell } from "./PreviewShell";
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

function prepareContent(content: unknown, truncationMessage?: string): PreparedContent {
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

  if (truncated && truncationMessage) {
    text += "\n\n... " + truncationMessage;
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
  const { t } = useTranslation();
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
          setError(err instanceof Error ? err.message : t("preview.textLoadFailed"));
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [file.id, file.size, reloadKey]);

  if (file.size > MAX_TEXT_BYTES) {
    return <PreviewUnsupported file={file} reason={t("preview.fileTooLarge")} />;
  }

  if (loading) return <PreviewLoading label={t("preview.loadingText")} />;
  if (error) {
    return (
      <PreviewError
        message={t("preview.textLoadFailed")}
        onRetry={() => setReloadKey((value) => value + 1)}
      />
    );
  }

  return <CodePreviewDocument file={file} content={content} />;
}

export function CodePreviewDocument({
  file,
  content,
}: {
  file: DriveFile;
  content: string;
}) {
  const { t } = useTranslation();
  const prepared = useMemo(() => prepareContent(content, t("preview.textTruncated")), [content, t]);
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
  const renderMarkdown = isMarkdownFile(file) && file.size < MARKDOWN_RENDER_BYTES;

  return (
    <PreviewShell
      className={styles.codeShell}
      mode="padded"
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
              {t("preview.contentTooLong")}
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
              {t("preview.contentTooLong")}
            </p>
          )}
        </>
      )}
    </PreviewShell>
  );
}
