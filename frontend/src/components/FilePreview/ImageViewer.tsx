import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type WheelEvent,
} from "react";
import { useTranslation } from "react-i18next";
import { downloadUrl, getMetadata } from "../../api/fileApi";
import type { DriveFile, MediaMeta } from "../../types";
import { MediaMetaBar } from "./MediaMetaBar";
import { PreviewShell } from "./PreviewShell";
import { PreviewToolbar } from "./PreviewToolbar";
import { PreviewError, PreviewLoading } from "./PreviewState";
import styles from "./FilePreview.module.css";

const MIN_SCALE = 0.25;
const MAX_SCALE = 4;

function clampScale(value: number) {
  return Math.min(MAX_SCALE, Math.max(MIN_SCALE, Number(value.toFixed(2))));
}

function withRetryParam(src: string, retryKey: number) {
  const sep = src.includes("?") ? "&" : "?";
  return `${src}${sep}preview_retry=${retryKey}`;
}

function formatDate(value?: string) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function ImageMetaPanel({
  meta,
  loading,
  fallbackDimensions,
  metaError,
}: {
  meta: MediaMeta | null;
  loading: boolean;
  fallbackDimensions: { width?: number; height?: number };
  metaError: string;
}) {
  const { t } = useTranslation();
  const takenAt = formatDate(meta?.taken_at);
  const hasLocation =
    typeof meta?.latitude === "number" && typeof meta?.longitude === "number";

  return (
    <div className={styles.imageMeta}>
      <h3>{t("preview.imageInfo")}</h3>
      <MediaMetaBar
        mode="image"
        meta={meta}
        loading={loading}
        fallbackDimensions={fallbackDimensions}
      />
      <dl className={styles.metaList}>
        {takenAt && (
          <div>
            <dt>{t("preview.takenAt")}</dt>
            <dd>{takenAt}</dd>
          </div>
        )}
        {meta?.camera && (
          <div>
            <dt>{t("preview.camera")}</dt>
            <dd>{meta.camera}</dd>
          </div>
        )}
        {hasLocation && (
          <div>
            <dt>{t("preview.location")}</dt>
            <dd>
              <a
                className={styles.mapLink}
                href={`https://www.google.com/maps?q=${meta.latitude},${meta.longitude}`}
                target="_blank"
                rel="noreferrer"
              >
                {meta.latitude?.toFixed(6)}, {meta.longitude?.toFixed(6)}
              </a>
            </dd>
          </div>
        )}
        {!loading && metaError && (
          <div>
            <dt>元信息</dt>
            <dd>{t("preview.noMetaInfo")}</dd>
          </div>
        )}
      </dl>
    </div>
  );
}

export function ImageViewer({ file }: { file: DriveFile }) {
  const { t } = useTranslation();
  const [scale, setScale] = useState(1);
  const [fitMode, setFitMode] = useState(true);
  const [showMeta, setShowMeta] = useState(false);
  const [meta, setMeta] = useState<MediaMeta | null>(null);
  const [metaLoading, setMetaLoading] = useState(true);
  const [metaError, setMetaError] = useState("");
  const [loadFailed, setLoadFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [naturalSize, setNaturalSize] = useState({ width: 0, height: 0 });
  const [retryKey, setRetryKey] = useState(0);
  const bodyRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    bodyRef.current?.focus();
  }, [file.id]);

  useEffect(() => {
    let cancelled = false;
    setMetaLoading(true);
    setMetaError("");
    getMetadata(file.id)
      .then((value) => {
        if (!cancelled) setMeta(value);
      })
      .catch((err) => {
        if (!cancelled) {
          setMeta(null);
          setMetaError(err instanceof Error ? err.message : t("preview.metaLoadFailed"));
        }
      })
      .finally(() => {
        if (!cancelled) setMetaLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [file.id]);

  const src = useMemo(() => withRetryParam(downloadUrl(file.id), retryKey), [
    file.id,
    retryKey,
  ]);

  function zoom(next: number) {
    setFitMode(false);
    setScale(clampScale(next));
  }

  function retry() {
    setLoadFailed(false);
    setLoaded(false);
    setScale(1);
    setFitMode(true);
    setRetryKey((value) => value + 1);
  }

  function handleWheel(event: WheelEvent<HTMLDivElement>) {
    event.preventDefault();
    zoom(scale * (event.deltaY > 0 ? 0.9 : 1.1));
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === "+" || event.key === "=") {
      event.preventDefault();
      zoom(scale * 1.1);
    }
    if (event.key === "-") {
      event.preventDefault();
      zoom(scale * 0.9);
    }
    if (event.key === "0") {
      event.preventDefault();
      setScale(1);
      setFitMode(true);
    }
  }

  const isHugeImage =
    file.size > 10 * 1024 * 1024 ||
    naturalSize.width > 8000 ||
    naturalSize.height > 8000;

  return (
    <PreviewShell
      mode="fullbleed"
      bodyClassName={styles.imageBody}
      toolbar={
        <PreviewToolbar
          trailing={
            <button
              className={styles.metaToggle}
              type="button"
              onClick={() => setShowMeta((value) => !value)}
            >
              {showMeta ? t("preview.hideInfo") : t("preview.imageInfo")}
            </button>
          }
        >
          <button
            className={styles.toolButton}
            type="button"
            onClick={() => zoom(scale * 0.9)}
          >
            -
          </button>
          <span className={styles.pageIndicator}>{Math.round(scale * 100)}%</span>
          <button
            className={styles.toolButton}
            type="button"
            onClick={() => zoom(scale * 1.1)}
          >
            +
          </button>
          <button
            className={styles.toolButton}
            type="button"
            onClick={() => {
              setFitMode(false);
              setScale(1);
            }}
          >
            1:1
          </button>
          <button
            className={styles.toolButton}
            type="button"
            onClick={() => {
              setFitMode(true);
              setScale(1);
            }}
          >
            Fit
          </button>
        </PreviewToolbar>
      }
      meta={
        showMeta ? (
          <ImageMetaPanel
            meta={meta}
            loading={metaLoading}
            fallbackDimensions={{
              width: naturalSize.width || undefined,
              height: naturalSize.height || undefined,
            }}
            metaError={metaError}
          />
        ) : undefined
      }
    >
      <div
        ref={bodyRef}
        className={styles.imageStage}
        tabIndex={0}
        onWheel={handleWheel}
        onKeyDown={handleKeyDown}
        onDoubleClick={() => {
          setFitMode(true);
          setScale(1);
        }}
      >
        {!loaded && !loadFailed && <PreviewLoading label={t("preview.loadingImage")} />}
        {loadFailed ? (
          <PreviewError message={t("preview.imageLoadFailed")} onRetry={retry} />
        ) : (
          <img
            key={retryKey}
            className={styles.image}
            src={src}
            alt={file.name}
            loading={isHugeImage ? "lazy" : "eager"}
            decoding="async"
            style={{
              width: naturalSize.width ? `${naturalSize.width}px` : undefined,
              height: naturalSize.height ? `${naturalSize.height}px` : undefined,
              maxWidth: fitMode ? "100%" : "none",
              maxHeight: fitMode ? "100%" : "none",
              transform: `scale(${scale})`,
              display: loaded ? "block" : "none",
            }}
            onLoad={(event) => {
              setLoaded(true);
              setLoadFailed(false);
              setNaturalSize({
                width: event.currentTarget.naturalWidth,
                height: event.currentTarget.naturalHeight,
              });
            }}
            onError={() => {
              setLoaded(true);
              setLoadFailed(true);
            }}
          />
        )}
      </div>
    </PreviewShell>
  );
}
