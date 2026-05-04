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
  if (loading) {
    return <div className={styles.metaSkeleton}>读取媒体元信息...</div>;
  }

  const width = meta?.width || fallbackDimensions?.width;
  const height = meta?.height || fallbackDimensions?.height;
  const items: Array<{ label: string; value: string }> = [];

  if ((mode === "video" || mode === "image") && width && height) {
    items.push({ label: "分辨率", value: `${width} × ${height}` });
  }
  if ((mode === "video" || mode === "audio") && meta?.duration) {
    items.push({ label: "时长", value: formatDuration(meta.duration) });
  }
  if ((mode === "video" || mode === "audio") && meta?.codec) {
    items.push({ label: "编码", value: meta.codec });
  }
  const bitrate = formatBitrate(meta?.bitrate);
  if ((mode === "video" || mode === "audio") && bitrate) {
    items.push({ label: "比特率", value: bitrate });
  }
  if ((mode === "video" || mode === "audio") && meta?.format) {
    items.push({ label: "格式", value: meta.format });
  }

  if (items.length === 0) {
    return <div className={styles.metaEmpty}>暂无元信息</div>;
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
