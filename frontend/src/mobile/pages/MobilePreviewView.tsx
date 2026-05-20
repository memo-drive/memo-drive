import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { FilePreview } from "../../components/FilePreview/FilePreview";
import { PreviewChromeProvider } from "../../components/FilePreview/PreviewState";
import type { DriveFile } from "../../types";
import styles from "./MobilePreviewView.module.css";

interface MobilePreviewViewProps {
  file?: DriveFile;
  returnHref: string;
  downloadHref?: string;
  loading?: boolean;
  error?: string;
}

export function MobilePreviewView({
  file,
  returnHref,
  downloadHref,
  loading = false,
  error = "",
}: MobilePreviewViewProps) {
  const { t } = useTranslation();

  return (
    <section className={styles.page}>
      <header className={styles.header}>
        <Link className={styles.iconButton} to={returnHref}>
          <span className="material-symbols-outlined" aria-hidden>
            arrow_back
          </span>
        </Link>
        <div className={styles.titleGroup}>
          <h1>{file?.name ?? t("common.loading")}</h1>
        </div>
        {file && downloadHref ? (
          <a
            className={styles.iconButton}
            href={downloadHref}
            download={file.name}
            aria-label={t("common.download")}
          >
            <span className="material-symbols-outlined" aria-hidden>
              download
            </span>
          </a>
        ) : (
          <span className={styles.iconButtonPlaceholder} />
        )}
      </header>

      <main className={styles.previewArea}>
        {loading && !file ? (
          <div className={styles.state}>{t("common.loading")}</div>
        ) : error ? (
          <div className={styles.state}>{error}</div>
        ) : (
          <PreviewChromeProvider showEyebrow={false}>
            <FilePreview file={file} />
          </PreviewChromeProvider>
        )}
      </main>
    </section>
  );
}
