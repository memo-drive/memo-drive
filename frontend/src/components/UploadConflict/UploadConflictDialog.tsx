import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, Modal } from "../base";
import type { PendingUploadConflict } from "../../hooks/useUploadConflictResolver";
import type { UploadConflictAction } from "../../workflows/uploadConflictWorkflow";
import styles from "./UploadConflictDialog.module.css";

interface UploadConflictDialogProps {
  conflict: PendingUploadConflict | null;
  onDecision: (
    action: UploadConflictAction,
    applyToRemaining: boolean,
  ) => void;
}

export function UploadConflictDialog({
  conflict,
  onDecision,
}: UploadConflictDialogProps) {
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
    <Modal
      open
      closable={false}
      width={560}
      title={t("uploadConflict.title")}
      onClose={() => decide("skip")}
    >
      <div className={styles.summary}>
        <span className={`material-symbols-outlined ${styles.summaryIcon}`} aria-hidden>
          file_copy
        </span>
        <div>
          <p className={styles.progress}>
            {t("uploadConflict.progress", {
              current: conflict.current,
              total: conflict.total,
            })}
          </p>
          <h3>{conflict.item.requested_name}</h3>
          <p>{t("uploadConflict.description")}</p>
        </div>
      </div>

      <div className={styles.renameHint}>
        <span>{t("uploadConflict.keepBothHint")}</span>
        <strong>
          {conflict.item.rename_suggestion ??
            t("uploadConflict.renameFallback")}
        </strong>
      </div>

      <label className={styles.applyRow}>
        <input
          type="checkbox"
          checked={applyToRemaining}
          onChange={(event) => setApplyToRemaining(event.target.checked)}
        />
        <span>{t("uploadConflict.applyToRemaining")}</span>
      </label>

      <div className={styles.actions}>
        <Button
          variant="primary"
          size="lg"
          autoFocus
          onClick={() => decide("rename")}
        >
          {t("uploadConflict.keepBoth")}
        </Button>
        {conflict.item.replace_allowed ? (
          <Button
            variant="secondary"
            size="lg"
            onClick={() => decide("replace")}
          >
            {t("uploadConflict.replace")}
          </Button>
        ) : null}
        <Button variant="ghost" size="lg" onClick={() => decide("skip")}>
          {t("uploadConflict.skip")}
        </Button>
      </div>
      {conflict.item.replace_allowed ? (
        <p className={styles.warning}>{t("uploadConflict.replaceWarning")}</p>
      ) : null}
    </Modal>
  );
}
