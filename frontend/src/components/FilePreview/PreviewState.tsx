import { createContext, useContext, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { downloadUrl } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { formatBytes } from "../../utils/formatBytes";
import styles from "./FilePreview.module.css";

export { formatBytes };

const PreviewChromeContext = createContext({ showEyebrow: true });

export function PreviewChromeProvider({
  children,
  showEyebrow,
}: {
  children: ReactNode;
  showEyebrow: boolean;
}) {
  return (
    <PreviewChromeContext.Provider value={{ showEyebrow }}>
      {children}
    </PreviewChromeContext.Provider>
  );
}

export function usePreviewChrome() {
  return useContext(PreviewChromeContext);
}

export function PreviewLoading({ label }: { label?: string }) {
  const { t } = useTranslation();
  const { showEyebrow } = usePreviewChrome();
  return (
    <div className={styles.placeholder}>
      <span className={styles.spinner} aria-hidden="true" />
      {showEyebrow ? <p className={styles.eyebrow}>Loading</p> : null}
      <h3 className={styles.title}>{label ?? t("preview.loading")}</h3>
      <p className={styles.desc}>{t("preview.moving")}</p>
    </div>
  );
}

export function PreviewError({
  message,
  onRetry,
}: {
  message: string;
  onRetry?: () => void;
}) {
  const { t } = useTranslation();
  const { showEyebrow } = usePreviewChrome();
  return (
    <div className={styles.placeholder}>
      <div className={styles.errorBar} />
      {showEyebrow ? <p className={styles.eyebrow}>Preview Error</p> : null}
      <h3 className={styles.title}>{message}</h3>
      <p className={styles.desc}>{t("preview.retryHint")}</p>
      {onRetry && (
        <button className={styles.retryBtn} type="button" onClick={onRetry}>
          {t("preview.reload")}
        </button>
      )}
    </div>
  );
}

export function PreviewUnsupported({
  file,
  reason,
}: {
  file: DriveFile;
  reason?: string;
}) {
  const { t } = useTranslation();
  const { showEyebrow } = usePreviewChrome();
  return (
    <div className={styles.placeholder}>
      {showEyebrow ? <p className={styles.eyebrow}>Unsupported</p> : null}
      <h3 className={styles.title}>{file.name}</h3>
      <p className={styles.desc}>{reason ?? t("preview.notSupported")}</p>
      <dl className={styles.unsupportedMeta}>
        <div>
          <dt>{t("preview.type")}</dt>
          <dd>{file.mime_type || t("preview.unknownType")}</dd>
        </div>
        <div>
          <dt>大小</dt>
          <dd>{formatBytes(file.size)}</dd>
        </div>
      </dl>
      <button
        className={styles.retryBtn}
        type="button"
        onClick={() => window.open(downloadUrl(file.id), "_blank", "noreferrer")}
      >
        {t("preview.downloadFile")}
      </button>
    </div>
  );
}
