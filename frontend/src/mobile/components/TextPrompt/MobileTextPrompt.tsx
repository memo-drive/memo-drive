import { type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import styles from "./MobileTextPrompt.module.css";

interface MobileTextPromptProps {
  open: boolean;
  title: string;
  label: string;
  value: string;
  confirmText: string;
  cancelText?: string;
  error?: string;
  busy?: boolean;
  disabled?: boolean;
  onValueChange: (value: string) => void;
  onCancel: () => void;
  onConfirm: () => void;
}

export function MobileTextPrompt({
  open,
  title,
  label,
  value,
  confirmText,
  cancelText,
  error = "",
  busy = false,
  disabled = false,
  onValueChange,
  onCancel,
  onConfirm,
}: MobileTextPromptProps) {
  const { t } = useTranslation();

  if (!open) return null;

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!disabled && !busy) onConfirm();
  }

  return (
    <section
      className={styles.overlay}
      role="dialog"
      aria-modal="true"
      aria-label={title}
      data-mobile-text-prompt="light"
    >
      <button className={styles.backdrop} type="button" onClick={onCancel} aria-label={t("common.close")} />
      <form className={styles.panel} onSubmit={submit}>
        <div className={styles.handle} />
        <header className={styles.header}>
          <h2>{title}</h2>
          <button type="button" onClick={onCancel} aria-label={t("common.close")}>
            <span className="material-symbols-outlined" aria-hidden>
              close
            </span>
          </button>
        </header>
        <label className={styles.field}>
          <span>{label}</span>
          <input
            value={value}
            disabled={busy}
            autoFocus
            onChange={(event) => onValueChange(event.target.value)}
          />
        </label>
        {error ? <p className={styles.error}>{error}</p> : null}
        <div className={styles.actions}>
          <button className={styles.cancelButton} type="button" disabled={busy} onClick={onCancel}>
            {cancelText ?? t("common.cancel")}
          </button>
          <button className={styles.confirmButton} type="submit" disabled={disabled || busy}>
            {confirmText}
          </button>
        </div>
      </form>
    </section>
  );
}
