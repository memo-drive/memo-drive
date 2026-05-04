import { downloadUrl } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import styles from "./FilePreview.module.css";

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value >= 10 || unit === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unit]}`;
}

export function PreviewLoading({ label = "加载中..." }: { label?: string }) {
  return (
    <div className={styles.placeholder}>
      <span className={styles.spinner} aria-hidden="true" />
      <p className={styles.eyebrow}>Loading</p>
      <h3 className={styles.title}>{label}</h3>
      <p className={styles.desc}>正在把文件搬到预览窗口里。</p>
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
  return (
    <div className={styles.placeholder}>
      <div className={styles.errorBar} />
      <p className={styles.eyebrow}>Preview Error</p>
      <h3 className={styles.title}>{message}</h3>
      <p className={styles.desc}>可以稍后重试，或直接下载原文件查看。</p>
      {onRetry && (
        <button className={styles.retryBtn} type="button" onClick={onRetry}>
          重新加载
        </button>
      )}
    </div>
  );
}

export function PreviewUnsupported({
  file,
  reason = "暂不支持在线预览",
}: {
  file: DriveFile;
  reason?: string;
}) {
  return (
    <div className={styles.placeholder}>
      <p className={styles.eyebrow}>Unsupported</p>
      <h3 className={styles.title}>{file.name}</h3>
      <p className={styles.desc}>{reason}</p>
      <dl className={styles.unsupportedMeta}>
        <div>
          <dt>类型</dt>
          <dd>{file.mime_type || "未知类型"}</dd>
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
        下载文件
      </button>
    </div>
  );
}
