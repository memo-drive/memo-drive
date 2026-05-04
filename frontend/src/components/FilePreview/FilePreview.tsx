import { lazy, Suspense } from "react";
import type { DriveFile } from "../../types";
import { AudioPlayer } from "./AudioPlayer";
import { CodeViewer } from "./CodeViewer";
import { ImageViewer } from "./ImageViewer";
import { PreviewLoading, PreviewUnsupported } from "./PreviewState";
import { isTextLikeFile } from "./textTypes";
import { VideoPlayer } from "./VideoPlayer";
import styles from "./FilePreview.module.css";

const PdfViewer = lazy(() =>
  import("./PdfViewer").then((module) => ({ default: module.PdfViewer })),
);

export function FilePreview({ file }: { file?: DriveFile }) {
  if (!file) {
    return (
      <section className={styles.placeholder}>
        <p className={styles.eyebrow}>Preview</p>
        <h3 className={styles.title}>选择文件后在这里预览</h3>
        <p className={styles.desc}>图片、视频、PDF 会直接展示，其他文件提供下载入口。</p>
      </section>
    );
  }
  if (file.is_dir) {
    return (
      <section className={styles.placeholder}>
        <p className={styles.eyebrow}>Folder</p>
        <h3 className={styles.title}>{file.name}</h3>
        <p className={styles.desc}>双击文件夹进入目录。</p>
      </section>
    );
  }
  if (file.mime_type.startsWith("image/")) return <ImageViewer file={file} />;
  if (file.mime_type.startsWith("video/")) return <VideoPlayer file={file} />;
  if (file.mime_type.startsWith("audio/")) return <AudioPlayer file={file} />;
  if (file.mime_type === "application/pdf") {
    return (
      <Suspense fallback={<PreviewLoading label="加载 PDF 预览器..." />}>
        <PdfViewer file={file} />
      </Suspense>
    );
  }
  if (isTextLikeFile(file)) return <CodeViewer file={file} />;
  return <PreviewUnsupported file={file} />;
}
