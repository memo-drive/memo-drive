import { useState, type KeyboardEvent } from "react";
import { useTranslation } from "react-i18next";
import { getFile } from "../../api/fileApi";
import { message } from "../base";
import { useFileStore } from "../../stores/fileStore";
import type { SourceChunk } from "../../types";
import styles from "./SourceReference.module.css";

interface SourceReferenceProps {
  sources: SourceChunk[];
  title?: string;
  compact?: boolean;
  loading?: boolean;
  onOpenSource?: (source: SourceChunk) => void | Promise<void>;
}

export function SourceReference({
  sources,
  title,
  compact = false,
  loading = false,
  onOpenSource,
}: SourceReferenceProps) {
  const { t } = useTranslation();
  const displayTitle = title ?? t("ai.sourceTitle");
  const setCurrentPath = useFileStore((state) => state.setCurrentPath);
  const setSelectedFile = useFileStore((state) => state.setSelectedFile);
  const [disabledIDs, setDisabledIDs] = useState<Set<string>>(() => new Set());

  if (loading) {
    return <div className={styles.loading}>{t("ai.sourceLoading")}</div>;
  }
  if (!sources.length) return null;

  async function openSource(source: SourceChunk) {
    if (disabledIDs.has(source.id)) return;
    if (onOpenSource) {
      await onOpenSource(source);
      return;
    }
    try {
      const file = await getFile(source.file_id);
      setCurrentPath(file.path || "/");
      setSelectedFile(file);
    } catch {
      setDisabledIDs((previous) => new Set(previous).add(source.id));
      message.error(t("ai.sourceMissing"));
    }
  }

  function onKeyDown(event: KeyboardEvent<HTMLDivElement>, source: SourceChunk) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      void openSource(source);
    }
  }

  return (
    <section className={`${styles.wrapper} ${compact ? styles.compact : ""}`}>
      <div className={styles.header}>
        <span>{displayTitle}</span>
        <span className={styles.count}>{sources.length}</span>
      </div>
      <div className={styles.list}>
        {sources.map((source, index) => {
          const disabled = disabledIDs.has(source.id);
          return (
            <div
              key={`${source.id}-${index}`}
              className={`${styles.card} ${disabled ? styles.disabled : ""}`}
              role="button"
              tabIndex={disabled ? -1 : 0}
              aria-disabled={disabled}
              onClick={() => void openSource(source)}
              onKeyDown={(event) => onKeyDown(event, source)}
            >
              <div className={styles.cardTop}>
                <span className={styles.index}>[{index + 1}]</span>
                <span className={styles.fileName}>{source.file_name || t("ai.sourceUnknownFile")}</span>
                <span className={styles.score}>{Math.round(source.score * 100)}%</span>
              </div>
              {source.heading ? <div className={styles.heading}>{source.heading}</div> : null}
              <p className={styles.snippet}>{source.snippet || source.text}</p>
            </div>
          );
        })}
      </div>
    </section>
  );
}
