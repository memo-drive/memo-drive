import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { DriveFile, MediaMeta } from "../../types";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import {
  mediaSwipeNavigation,
  type MediaSwipePoint,
  type MobileMediaCategory,
} from "./mobileMediaPreviewActions";
import styles from "./MobileMediaPreviewView.module.css";

interface MobileMediaPreviewViewProps {
  category: MobileMediaCategory;
  file?: DriveFile;
  returnHref: string;
  queue?: DriveFile[];
  queuePosition?: { current: number; total: number };
  canGoPrevious?: boolean;
  canGoNext?: boolean;
  downloadHref?: string;
  posterHref?: string;
  meta?: MediaMeta | null;
  metaLoading?: boolean;
  metaError?: string;
  moreOpen?: boolean;
  deleteConfirmOpen?: boolean;
  deleting?: boolean;
  loading?: boolean;
  error?: string;
  onPrevious?: () => void;
  onNext?: () => void;
  onOpenMore?: () => void;
  onCloseMore?: () => void;
  onDelete?: () => void;
  onCancelDelete?: () => void;
  onConfirmDelete?: () => void;
  onSelectQueueFile?: (file: DriveFile) => void;
}

export function MobileMediaPreviewView({
  category,
  file,
  returnHref,
  queue = [],
  queuePosition,
  canGoPrevious = false,
  canGoNext = false,
  downloadHref,
  posterHref,
  meta,
  metaLoading = false,
  metaError = "",
  moreOpen = false,
  deleteConfirmOpen = false,
  deleting = false,
  loading = false,
  error = "",
  onPrevious,
  onNext,
  onOpenMore,
  onCloseMore,
  onDelete,
  onCancelDelete,
  onConfirmDelete,
  onSelectQueueFile,
}: MobileMediaPreviewViewProps) {
  const { t } = useTranslation();
  const [chromeVisible, setChromeVisible] = useState(true);
  const [photoZoomed, setPhotoZoomed] = useState(false);
  const [playbackError, setPlaybackError] = useState(false);
  const swipeStartRef = useRef<MediaSwipePoint | null>(null);
  const suppressStageClickRef = useRef(false);
  const mediaKind = mediaKindFor(category);
  const title = mediaKind === "audio" ? t("mobile.category.audio.title") : file?.name ?? t("mobile.media.title");
  const showAudioQueue = mediaKind === "audio" && queue.length > 1;
  const rows = metaRows(meta, t);

  useEffect(() => {
    setChromeVisible(true);
    setPhotoZoomed(false);
    setPlaybackError(false);
  }, [file?.id]);

  return (
    <section
      className={styles.page}
      data-mobile-page="media-preview"
      data-mobile-media-kind={mediaKind}
      data-media-chrome-visible={chromeVisible}
    >
      <header className={`${styles.topBar} ${chromeVisible ? "" : styles.topBarHidden}`}>
        <Link className={styles.iconButton} to={returnHref} aria-label={t("common.back")}>
          <span className="material-symbols-outlined" aria-hidden>
            arrow_back
          </span>
        </Link>
        <div className={styles.titleGroup}>
          <h1>{title}</h1>
          <small>{queuePosition ? `${queuePosition.current} / ${queuePosition.total}` : t("mobile.media.title")}</small>
        </div>
        <button
          className={styles.iconButton}
          type="button"
          aria-label={t("mobile.media.more")}
          onClick={onOpenMore}
        >
          <span className="material-symbols-outlined" aria-hidden>
            more_horiz
          </span>
        </button>
      </header>
      <main
        className={styles.stage}
        data-swipe-navigation="true"
        onPointerDown={(event) => {
          if (event.pointerType === "mouse" && event.button !== 0) return;
          swipeStartRef.current = { x: event.clientX, y: event.clientY };
        }}
        onPointerCancel={() => {
          swipeStartRef.current = null;
        }}
        onPointerUp={(event) => {
          const start = swipeStartRef.current;
          swipeStartRef.current = null;
          if (!start) return;

          const navigation = mediaSwipeNavigation(start, { x: event.clientX, y: event.clientY });
          if (!navigation) return;

          suppressStageClickRef.current = true;
          if (navigation === "previous" && canGoPrevious) onPrevious?.();
          if (navigation === "next" && canGoNext) onNext?.();
        }}
        onClick={() => {
          if (suppressStageClickRef.current) {
            suppressStageClickRef.current = false;
            return;
          }
          if (mediaKind === "photo") setChromeVisible((value) => !value);
        }}
        onDoubleClick={(event) => {
          if (mediaKind !== "photo") return;
          event.preventDefault();
          setPhotoZoomed((value) => !value);
        }}
      >
        {loading && !file ? (
          <div className={styles.state}>{t("common.loading")}</div>
        ) : error ? (
          <div className={styles.state}>{error}</div>
        ) : file && mediaKind === "photo" ? (
          <img
            className={`${styles.photoImage} ${photoZoomed ? styles.photoZoomed : ""}`}
            src={downloadHref}
            alt={file.name}
            data-photo-zoomed={photoZoomed}
            draggable={false}
          />
        ) : file && mediaKind === "video" ? (
          playbackError ? (
            <div className={styles.state}>{t("preview.videoError")}</div>
          ) : (
            <video
              key={file.id}
              className={styles.videoPlayer}
              src={downloadHref}
              poster={posterHref}
              controls
              playsInline
              preload="metadata"
              data-media-player-key={file.id}
              onError={() => setPlaybackError(true)}
            />
          )
        ) : file && mediaKind === "audio" ? (
          <div className={styles.audioPanel}>
            <div className={styles.audioCard}>
              <span className="material-symbols-outlined" aria-hidden>
                headphones
              </span>
              <strong>{file.name}</strong>
              {playbackError ? (
                <div className={styles.state}>{t("preview.audioError")}</div>
              ) : (
                <audio
                  key={file.id}
                  className={styles.audioPlayer}
                  src={downloadHref}
                  controls
                  preload="metadata"
                  data-media-player-key={file.id}
                  onError={() => setPlaybackError(true)}
                />
              )}
            </div>
            {showAudioQueue ? (
              <ol className={styles.audioQueue} data-audio-queue="true">
                {queue.map((track) => {
                  const active = track.id === file.id;
                  return (
                    <li key={track.id}>
                      <button
                        type="button"
                        aria-current={active ? "true" : undefined}
                        onClick={() => onSelectQueueFile?.(track)}
                      >
                        <span className="material-symbols-outlined" aria-hidden>
                          {active ? "equalizer" : "music_note"}
                        </span>
                        <span>{track.name}</span>
                      </button>
                    </li>
                  );
                })}
              </ol>
            ) : null}
          </div>
        ) : null}
      </main>
      {file && moreOpen ? (
        <section className={styles.moreOverlay} aria-label={t("mobile.media.info")}>
          <button
            className={styles.moreBackdrop}
            type="button"
            aria-label={t("common.close")}
            onClick={onCloseMore}
          />
          <aside className={styles.morePanel} role="dialog" aria-modal="true" aria-label={t("mobile.media.info")}>
            <header>
              <h2>{t("mobile.media.info")}</h2>
              <button className={styles.moreClose} type="button" aria-label={t("common.close")} onClick={onCloseMore}>
                <span className="material-symbols-outlined" aria-hidden>
                  close
                </span>
              </button>
            </header>
            <dl className={styles.metaList}>
              {rows.map((row) => (
                <div key={row.label}>
                  <dt>{row.label}</dt>
                  <dd>{row.value}</dd>
                </div>
              ))}
              {metaLoading ? (
                <div>
                  <dt>{t("preview.metaInfo")}</dt>
                  <dd>{t("preview.mediaReading")}</dd>
                </div>
              ) : null}
              {!metaLoading && rows.length === 0 ? (
                <div>
                  <dt>{t("preview.metaInfo")}</dt>
                  <dd>{metaError || t("preview.noMetaInfo")}</dd>
                </div>
              ) : null}
            </dl>
            <div className={styles.moreActions}>
              {downloadHref ? (
                <a href={downloadHref} download={file.name}>
                  <span className="material-symbols-outlined" aria-hidden>
                    download
                  </span>
                  {t("common.download")}
                </a>
              ) : null}
              <button type="button" onClick={onDelete}>
                <span className="material-symbols-outlined" aria-hidden>
                  delete
                </span>
                {t("common.delete")}
              </button>
            </div>
          </aside>
        </section>
      ) : null}
      <MobileConfirmPrompt
        open={Boolean(file && deleteConfirmOpen)}
        title={t("drive.confirmDelete")}
        description={t("drive.deleteConfirmBody", { name: file?.name ?? "" })}
        confirmText={t("drive.deleteToTrash")}
        tone="danger"
        busy={deleting}
        onCancel={() => onCancelDelete?.()}
        onConfirm={() => onConfirmDelete?.()}
      />
    </section>
  );
}

function mediaKindFor(category: MobileMediaCategory) {
  if (category === "photos") return "photo";
  if (category === "videos") return "video";
  return "audio";
}

function metaRows(meta: MediaMeta | null | undefined, t: (key: string) => string) {
  if (!meta) return [];
  const rows: Array<{ label: string; value: string }> = [];
  if (meta.width && meta.height) {
    rows.push({ label: t("preview.resolution"), value: `${meta.width} x ${meta.height}` });
  }
  if (typeof meta.duration === "number") {
    rows.push({
      label: t("preview.duration"),
      value: `${Math.round(meta.duration)} ${t("preview.durationSeconds")}`,
    });
  }
  if (meta.camera) rows.push({ label: t("preview.camera"), value: meta.camera });
  if (meta.codec) rows.push({ label: t("preview.encoding"), value: meta.codec });
  if (meta.format) rows.push({ label: t("preview.format"), value: meta.format });
  if (typeof meta.bitrate === "number") rows.push({ label: t("preview.bitrate"), value: `${meta.bitrate}` });
  if (meta.taken_at) rows.push({ label: t("preview.takenAt"), value: meta.taken_at });
  return rows;
}
