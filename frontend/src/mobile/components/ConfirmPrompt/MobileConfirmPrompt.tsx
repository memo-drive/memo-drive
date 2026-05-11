import { useTranslation } from "react-i18next";
import styles from "./MobileConfirmPrompt.module.css";

interface MobileConfirmPromptProps {
  open: boolean;
  title: string;
  description: string;
  confirmText: string;
  cancelText?: string;
  tone?: "default" | "danger";
  busy?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

export function MobileConfirmPrompt({
  open,
  title,
  description,
  confirmText,
  cancelText,
  tone = "default",
  busy = false,
  onCancel,
  onConfirm,
}: MobileConfirmPromptProps) {
  const { t } = useTranslation();

  if (!open) return null;

  return (
    <section
      className={styles.overlay}
      role="alertdialog"
      aria-modal="true"
      aria-label={title}
      data-mobile-confirm="light"
    >
      <button className={styles.backdrop} type="button" onClick={onCancel} aria-label={t("common.close")} />
      <div className={styles.prompt}>
        <span
          className={`${styles.icon} ${tone === "danger" ? styles.iconDanger : ""} material-symbols-outlined`}
          aria-hidden
        >
          {tone === "danger" ? "delete" : "info"}
        </span>
        <div className={styles.copy}>
          <h2>{title}</h2>
          <p>{description}</p>
        </div>
        <div className={styles.actions}>
          <button className={styles.cancelButton} type="button" disabled={busy} onClick={onCancel}>
            {cancelText ?? t("common.cancel")}
          </button>
          <button
            className={`${styles.confirmButton} ${tone === "danger" ? styles.confirmDanger : ""}`}
            type="button"
            disabled={busy}
            onClick={onConfirm}
          >
            {confirmText}
          </button>
        </div>
      </div>
    </section>
  );
}
