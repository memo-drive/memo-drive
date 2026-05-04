import { useEffect, useState } from "react";
import { downloadUrl, getMetadata } from "../../api/fileApi";
import type { DriveFile, MediaMeta } from "../../types";
import { MediaMetaBar } from "./MediaMetaBar";
import { PreviewShell } from "./PreviewShell";
import { PreviewError } from "./PreviewState";
import styles from "./FilePreview.module.css";

export function AudioPlayer({ file }: { file: DriveFile }) {
  const [meta, setMeta] = useState<MediaMeta | null>(null);
  const [loadingMeta, setLoadingMeta] = useState(true);
  const [playbackError, setPlaybackError] = useState(false);

  useEffect(() => {
    let cancelled = false;
    setLoadingMeta(true);
    getMetadata(file.id)
      .then((value) => {
        if (!cancelled) setMeta(value);
      })
      .catch(() => {
        if (!cancelled) setMeta(null);
      })
      .finally(() => {
        if (!cancelled) setLoadingMeta(false);
      });
    return () => {
      cancelled = true;
    };
  }, [file.id]);

  return (
    <PreviewShell mode="padded">
      <div className={styles.mediaStack}>
        {playbackError ? (
          <PreviewError
            message="音频无法播放（可能编码不支持）"
            onRetry={() => setPlaybackError(false)}
          />
        ) : (
          <audio
            className={styles.audio}
            src={downloadUrl(file.id)}
            controls
            preload="metadata"
            onError={() => setPlaybackError(true)}
          />
        )}
        <MediaMetaBar mode="audio" meta={meta} loading={loadingMeta} />
      </div>
    </PreviewShell>
  );
}
