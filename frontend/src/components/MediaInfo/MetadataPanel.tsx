import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getMetadata } from "../../api/fileApi";
import type { DriveFile, MediaMeta } from "../../types";
import styles from "./MetadataPanel.module.css";

interface Props {
  file?: DriveFile;
}

export function MetadataPanel({ file }: Props) {
  const { t } = useTranslation();
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
        setError(t("media.noMeta"));
      });
  }, [file]);

  if (!file) return <aside className={`${styles.panel} ${styles.muted}`}>{t("media.selectToView")}</aside>;
  if (error) return <aside className={`${styles.panel} ${styles.muted}`}>{error}</aside>;
  if (!meta) return <aside className={`${styles.panel} ${styles.muted}`}>{t("media.metaParsing")}</aside>;

  const rows = [
    [t("media.dimensions"), meta.width && meta.height ? `${meta.width} x ${meta.height}` : ""],
    [t("media.duration"), meta.duration ? `${meta.duration.toFixed(1)} 秒` : ""],
    [t("media.takenAt"), meta.taken_at ? new Date(meta.taken_at).toLocaleString() : ""],
    [t("media.camera"), meta.camera ?? ""],
    [t("media.location"), meta.latitude && meta.longitude ? `${meta.latitude.toFixed(5)}, ${meta.longitude.toFixed(5)}` : ""],
    [t("media.encoding"), meta.codec ?? ""],
    [t("media.bitrate"), meta.bitrate ? `${Math.round(meta.bitrate / 1000)} kbps` : ""],
    [t("media.format"), meta.format ?? ""],
  ].filter(([, value]) => value);

  return (
    <aside className={styles.panel}>
      <p className={styles.eyebrow}>Metadata</p>
      <h3 className={styles.title}>{file.name}</h3>
      {rows.length === 0 ? (
        <p className={styles.muted}>{t("media.parsedEmpty")}</p>
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
