import { useEffect, useRef, type MouseEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import styles from "./Modal.module.css";

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  width?: number | string;
  height?: number | string;
  closable?: boolean;
  destroyOnClose?: boolean;
}

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  width = 520,
  height,
  closable = true,
  destroyOnClose = true,
}: ModalProps) {
  const { t } = useTranslation();
  const panelRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: Event) => {
      if ((e as KeyboardEvent).key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = "";
    };
  }, [open, onClose]);

  if (!open && destroyOnClose) return null;

  if (!open) return null;

  const handleOverlayClick = (e: MouseEvent<HTMLDivElement>) => {
    if (e.target === e.currentTarget) onClose();
  };

  return (
    <div className={styles.overlay} onClick={handleOverlayClick}>
      <div
        ref={panelRef}
        className={styles.panel}
        role="dialog"
        aria-modal="true"
        aria-label={typeof title === "string" ? title : undefined}
        style={{ width, height }}
      >
        {title && (
          <div className={styles.header}>
            <h2 className={styles.title}>{title}</h2>
            {closable && (
              <button
                className={styles.closeBtn}
                onClick={onClose}
                aria-label={t("common.close")}
              >
                ×
              </button>
            )}
          </div>
        )}
        {children && <div className={styles.body}>{children}</div>}
        {footer && <div className={styles.footer}>{footer}</div>}
      </div>
    </div>
  );
}
