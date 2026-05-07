import { useTranslation } from "react-i18next";
import type { MediaMeta } from "../../types";
import { formatDuration } from "./formatDuration";
import styles from "./FilePreview.module.css";

interface MediaMetaBarProps {
  meta: MediaMeta | null;
  loading?: boolean;
  mode: "video" | "audio" | "image";
  fallbackDimensions?: { width?: number; height?: number };
}

function formatBitrate(value?: number): string | null {
  if (!value || value <= 0) return null;
  return `${Math.round(value / 1000)} kbps`;
}

export function MediaMetaBar({
  meta,
  loading = false,
  mode,
  fallbackDimensions,
}: MediaMetaBarProps) {
  const { t } = useTranslation();

  if (loading) {
    return <div className={styles.metaSkeleton}>{t("preview.mediaReading")}</div>;
  }

  const width = meta?.width || fallbackDimensions?.width;
  const height = meta?.height || fallbackDimensions?.height;
  const items: Array<{ label: string; value: string }> = [];

  if ((mode === "video" || mode === "image") && width && height) {
    items.push({ label: t("preview.resolution"), value: `${width} × ${height}` });
  }
  if ((mode === "video" || mode === "audio") && meta?.duration) {
    items.push({ label: t("preview.duration"), value: formatDuration(meta.duration) });
  }
  if ((mode === "video" || mode === "audio") && meta?.codec) {
    items.push({ label: t("preview.encoding"), value: meta.codec });
  }
  const bitrate = formatBitrate(meta?.bitrate);
  if ((mode === "video" || mode === "audio") && bitrate) {
    items.push({ label: t("preview.bitrate"), value: bitrate });
  }
  if ((mode === "video" || mode === "audio") && meta?.format) {
    items.push({ label: t("preview.format"), value: meta.format });
  }

  if (items.length === 0) {
    return <div className={styles.metaEmpty}>{t("preview.noMetaInfo")}</div>;
  }

  return (
    <dl className={styles.metaBar}>
      {items.map((item) => (
        <div key={item.label}>
          <dt>{item.label}</dt>
          <dd>{item.value}</dd>
        </div>
      ))}
    </dl>
  );
}
