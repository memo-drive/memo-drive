import { useCallback, useEffect, useMemo, useState } from "react";
import { Navigate, useLocation, useNavigate, useParams } from "react-router-dom";
import {
  deleteFile,
  downloadUrl,
  getFile,
  getMetadata,
  markFileViewed,
  queryFiles,
  thumbnailUrl,
} from "../../api/fileApi";
import { message } from "../../components/base";
import type { DriveFile, MediaMeta } from "../../types";
import {
  appendMediaQueuePage,
  buildMediaQueueRequest,
  isMobileMediaCategory,
  mediaDeleteFallback,
  mediaHref,
  mediaReturnHref,
  nextMediaTarget,
  previousMediaTarget,
  type MobileMediaCategory,
} from "./mobileMediaPreviewActions";
import { MobileMediaPreviewView } from "./MobileMediaPreviewView";

export function MobileMediaPreviewPage() {
  const params = useParams();

  if (!isMobileMediaCategory(params.category) || !params.fileId) {
    return <Navigate to="/m" replace />;
  }

  return <MobileMediaPreviewController category={params.category} fileId={params.fileId} />;
}

function MobileMediaPreviewController({
  category,
  fileId,
}: {
  category: MobileMediaCategory;
  fileId: string;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const returnHref = useMemo(() => mediaReturnHref(category, location.search), [category, location.search]);
  const [file, setFile] = useState<DriveFile | undefined>();
  const [queue, setQueue] = useState<DriveFile[]>([]);
  const [cursor, setCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(true);
  const [queueLoading, setQueueLoading] = useState(false);
  const [error, setError] = useState("");
  const [meta, setMeta] = useState<MediaMeta | null>(null);
  const [metaLoading, setMetaLoading] = useState(false);
  const [metaError, setMetaError] = useState("");
  const [moreOpen, setMoreOpen] = useState(false);
  const [deleteConfirmOpen, setDeleteConfirmOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const queueWithCurrent = useMemo(() => appendMediaQueuePage(queue, [], file), [file, queue]);
  const currentIndex = queueWithCurrent.findIndex((item) => item.id === fileId);
  const previous = previousMediaTarget(queueWithCurrent, fileId);
  const next = nextMediaTarget(queueWithCurrent, fileId);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError("");
    getFile(fileId)
      .then((nextFile) => {
        if (cancelled) return;
        setFile(nextFile);
        void markFileViewed(nextFile.id).catch(() => undefined);
      })
      .catch((err) => {
        if (cancelled) return;
        setFile(undefined);
        setError(err instanceof Error ? err.message : "Failed to load file");
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [fileId]);

  useEffect(() => {
    let cancelled = false;
    async function loadInitialQueue() {
      setQueueLoading(true);
      setQueue([]);
      setCursor("");
      setHasMore(false);

      try {
        let merged: DriveFile[] = [];
        let nextCursor = "";
        let nextHasMore = false;

        for (let page = 0; page < 5; page += 1) {
          const response = await queryFiles(
            buildMediaQueueRequest(category, { cursor: nextCursor || undefined }),
          );
          if (cancelled) return;

          const items = response.items ?? [];
          merged = appendMediaQueuePage(merged, items);
          nextCursor = response.next_cursor ?? "";
          nextHasMore = Boolean(response.has_more);

          if (items.some((item) => item.id === fileId) || !nextHasMore || !nextCursor) {
            break;
          }
        }

        setQueue(appendMediaQueuePage(merged, [], file));
        setCursor(nextCursor);
        setHasMore(nextHasMore);
      } catch (err) {
        if (cancelled) return;
        setQueue(file ? [file] : []);
        setCursor("");
        setHasMore(false);
      } finally {
        if (!cancelled) setQueueLoading(false);
      }
    }

    void loadInitialQueue();
    return () => {
      cancelled = true;
    };
  }, [category, file?.id, fileId]);

  useEffect(() => {
    let cancelled = false;
    setMeta(null);
    setMetaError("");
    setMetaLoading(Boolean(fileId));
    getMetadata(fileId)
      .then((value) => {
        if (!cancelled) setMeta(value);
      })
      .catch((err) => {
        if (!cancelled) setMetaError(err instanceof Error ? err.message : "");
      })
      .finally(() => {
        if (!cancelled) setMetaLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [fileId]);

  const loadMoreQueue = useCallback(async () => {
    if (!hasMore || !cursor || queueLoading) return queueWithCurrent;

    setQueueLoading(true);
    try {
      const response = await queryFiles(buildMediaQueueRequest(category, { cursor }));
      const nextQueue = appendMediaQueuePage(queueWithCurrent, response.items ?? [], file);
      setQueue(nextQueue);
      setCursor(response.next_cursor ?? "");
      setHasMore(Boolean(response.has_more));
      return nextQueue;
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed to load media queue");
      return queueWithCurrent;
    } finally {
      setQueueLoading(false);
    }
  }, [category, cursor, file, hasMore, queueLoading, queueWithCurrent]);

  function openMedia(target: DriveFile, replace = false) {
    navigate(mediaHref(category, target.id, returnHref), { replace });
  }

  async function goNext() {
    if (next) {
      openMedia(next);
      return;
    }
    if (!hasMore) return;
    const loaded = await loadMoreQueue();
    const loadedNext = nextMediaTarget(loaded, fileId);
    if (loadedNext) openMedia(loadedNext);
  }

  function goPrevious() {
    if (previous) openMedia(previous);
  }

  async function confirmDelete() {
    if (!file) return;
    setDeleting(true);
    try {
      await deleteFile(file.id);
      const fallback = mediaDeleteFallback(queueWithCurrent, file.id, category, returnHref);
      setDeleteConfirmOpen(false);
      setMoreOpen(false);
      setQueue((current) => current.filter((item) => item.id !== file.id));
      message.success(`${file.name} 已移到回收站`);
      navigate(fallback.href, { replace: true });
    } catch (err) {
      message.error(err instanceof Error ? err.message : "Failed to delete file");
    } finally {
      setDeleting(false);
    }
  }

  return (
    <MobileMediaPreviewView
      category={category}
      file={file}
      returnHref={returnHref}
      queue={queueWithCurrent}
      queuePosition={
        currentIndex >= 0
          ? { current: currentIndex + 1, total: queueWithCurrent.length + (hasMore ? 1 : 0) }
          : undefined
      }
      canGoPrevious={Boolean(previous)}
      canGoNext={Boolean(next || hasMore)}
      downloadHref={file ? downloadUrl(file.id) : undefined}
      posterHref={file?.metadata?.thumbnail_path ? thumbnailUrl(file.id) : undefined}
      meta={meta}
      metaLoading={metaLoading}
      metaError={metaError}
      moreOpen={moreOpen}
      deleteConfirmOpen={deleteConfirmOpen}
      deleting={deleting}
      loading={loading || queueLoading}
      error={error}
      onPrevious={goPrevious}
      onNext={() => void goNext()}
      onOpenMore={() => setMoreOpen(true)}
      onCloseMore={() => setMoreOpen(false)}
      onDelete={() => setDeleteConfirmOpen(true)}
      onCancelDelete={() => setDeleteConfirmOpen(false)}
      onConfirmDelete={() => void confirmDelete()}
      onSelectQueueFile={(target) => openMedia(target)}
    />
  );
}
