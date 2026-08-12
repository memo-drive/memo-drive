import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import type { PendingUploadConflict } from "../../../hooks/useUploadConflictResolver";
import type { UploadConflictAction } from "../../../workflows/uploadConflictWorkflow";
import styles from "./MobileUploadConflictSheet.module.css";

interface MobileUploadConflictSheetProps {
  conflict: PendingUploadConflict | null;
  onDecision: (
    action: UploadConflictAction,
    applyToRemaining: boolean,
  ) => void;
}

export function MobileUploadConflictSheet({
  conflict,
  onDecision,
}: MobileUploadConflictSheetProps) {
  const { t } = useTranslation();
  const [applyToRemaining, setApplyToRemaining] = useState(false);

  useEffect(() => {
    setApplyToRemaining(false);
  }, [conflict?.file]);

  if (!conflict) return null;

  const decide = (action: UploadConflictAction) => {
    onDecision(action, applyToRemaining);
  };

  return (
    <section
      className={styles.overlay}
      role="dialog"
      aria-modal="true"
      aria-label={t("uploadConflict.title")}
      data-mobile-upload-conflict="bottom-sheet"
    >
      <button
        className={styles.backdrop}
        type="button"
        onClick={() => decide("skip")}
        aria-label={t("common.close")}
      />
      <div className={styles.sheet}>
        <div className={styles.handle} aria-hidden />
        <header>
          <span className="material-symbols-outlined" aria-hidden>
            file_copy
          </span>
          <div>
            <small>
              {t("uploadConflict.progress", {
                current: conflict.current,
                total: conflict.total,
              })}
            </small>
            <h2>{t("uploadConflict.title")}</h2>
          </div>
        </header>
        <div className={styles.fileName}>{conflict.item.requested_name}</div>
        <p className={styles.description}>{t("uploadConflict.description")}</p>
        <p className={styles.suggestion}>
          <span>{t("uploadConflict.keepBothHint")}</span>
          <strong>
            {conflict.item.rename_suggestion ??
              t("uploadConflict.renameFallback")}
          </strong>
        </p>
        <label className={styles.applyRow}>
          <input
            type="checkbox"
            checked={applyToRemaining}
            onChange={(event) => setApplyToRemaining(event.target.checked)}
          />
          <span>{t("uploadConflict.applyToRemaining")}</span>
        </label>
        <div className={styles.actions}>
          <button
            className={styles.keepButton}
            type="button"
            autoFocus
            onClick={() => decide("rename")}
          >
            {t("uploadConflict.keepBoth")}
          </button>
          {conflict.item.replace_allowed ? (
            <button
              className={styles.replaceButton}
              type="button"
              onClick={() => decide("replace")}
            >
              {t("uploadConflict.replace")}
            </button>
          ) : null}
          <button
            className={styles.skipButton}
            type="button"
            onClick={() => decide("skip")}
          >
            {t("uploadConflict.skip")}
          </button>
        </div>
        {conflict.item.replace_allowed ? (
          <p className={styles.warning}>{t("uploadConflict.replaceWarning")}</p>
        ) : null}
      </div>
    </section>
  );
}
