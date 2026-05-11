import { lazy, Suspense } from "react";
import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../types";
import { AudioPlayer } from "./AudioPlayer";
import { CodeViewer } from "./CodeViewer";
import { ImageViewer } from "./ImageViewer";
import { PreviewLoading, PreviewUnsupported, usePreviewChrome } from "./PreviewState";
import { isTextLikeFile } from "./textTypes";
import { VideoPlayer } from "./VideoPlayer";
import styles from "./FilePreview.module.css";

const PdfViewer = lazy(() =>
  import("./PdfViewer").then((module) => ({ default: module.PdfViewer })),
);

export function FilePreview({ file }: { file?: DriveFile }) {
  const { t } = useTranslation();
  const { showEyebrow } = usePreviewChrome();

  if (!file) {
    return (
      <section className={styles.placeholder}>
        {showEyebrow ? <p className={styles.eyebrow}>Preview</p> : null}
        <h3 className={styles.title}>{t("preview.chooseHint")}</h3>
        <p className={styles.desc}>{t("preview.chooseSubHint")}</p>
      </section>
    );
  }
  if (file.is_dir) {
    return (
      <section className={styles.placeholder}>
        {showEyebrow ? <p className={styles.eyebrow}>Folder</p> : null}
        <h3 className={styles.title}>{file.name}</h3>
        <p className={styles.desc}>{t("preview.dblClickHint")}</p>
      </section>
    );
  }
  if (file.mime_type.startsWith("image/")) return <ImageViewer file={file} />;
  if (file.mime_type.startsWith("video/")) return <VideoPlayer file={file} />;
  if (file.mime_type.startsWith("audio/")) return <AudioPlayer file={file} />;
  if (file.mime_type === "application/pdf") {
    return (
      <Suspense fallback={<PreviewLoading label={t("preview.loadingPdf")} />}>
        <PdfViewer file={file} />
      </Suspense>
    );
  }
  if (isTextLikeFile(file)) return <CodeViewer file={file} />;
  return <PreviewUnsupported file={file} />;
}
