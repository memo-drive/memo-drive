import { useEffect, useState } from "react";
import { getMetadata } from "../../api/fileApi";
import type { DriveFile, MediaMeta } from "../../types";
import styles from "./MetadataPanel.module.css";

interface Props {
  file?: DriveFile;
}

export function MetadataPanel({ file }: Props) {
  const [meta, setMeta] = useState<MediaMeta | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!file || file.is_dir) {
      setMeta(null);
      return;
    }
    setError("");
    getMetadata(file.id)
      .then(setMeta)
      .catch(() => {
        setMeta(null);
        setError("暂无媒体元信息");
      });
  }, [file]);

  if (!file) return <aside className={`${styles.panel} ${styles.muted}`}>选择一个文件查看详情。</aside>;
  if (error) return <aside className={`${styles.panel} ${styles.muted}`}>{error}</aside>;
  if (!meta) return <aside className={`${styles.panel} ${styles.muted}`}>元信息解析中或不适用于该文件。</aside>;

  const rows = [
    ["尺寸", meta.width && meta.height ? `${meta.width} x ${meta.height}` : ""],
    ["时长", meta.duration ? `${meta.duration.toFixed(1)} 秒` : ""],
    ["拍摄时间", meta.taken_at ? new Date(meta.taken_at).toLocaleString() : ""],
    ["相机", meta.camera ?? ""],
    ["位置", meta.latitude && meta.longitude ? `${meta.latitude.toFixed(5)}, ${meta.longitude.toFixed(5)}` : ""],
    ["编码", meta.codec ?? ""],
    ["码率", meta.bitrate ? `${Math.round(meta.bitrate / 1000)} kbps` : ""],
    ["格式", meta.format ?? ""],
  ].filter(([, value]) => value);

  return (
    <aside className={styles.panel}>
      <p className={styles.eyebrow}>Metadata</p>
      <h3 className={styles.title}>{file.name}</h3>
      {rows.length === 0 ? (
        <p className={styles.muted}>已解析，但没有可展示的字段。</p>
      ) : (
        <dl className={styles.dl}>
          {rows.map(([label, value]) => (
            <div key={label} className={styles.row}>
              <dt>{label}</dt>
              <dd>{value}</dd>
            </div>
          ))}
        </dl>
      )}
    </aside>
  );
}
