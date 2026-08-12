import { useCallback, useEffect, useRef, useState } from "react";
import { Navigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import {
  batchDeleteFiles,
  batchMoveFiles,
  listFiles,
  listPhotoMonths,
  listRecentlyViewedFiles,
  queryFiles,
  queryPhotoTimeline,
} from "../../api/fileApi";
import { message } from "../../components/base";
import { appendFolderPage } from "../../workflows/folderPagination";
import type {
  DriveFile,
  FileQueryDocumentSubtype,
  FileQueryMediaFilter,
  PhotoMonthIndexItem,
} from "../../types";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import { mobileBatchResultFeedback } from "../selection/mobileBatchResult";
import { joinMobileMovePath, mobileMoveDisabledReason } from "../selection/mobileMoveTarget";
import { MobileMoveTargetPrompt } from "../selection/MobileMoveTargetPrompt";
import {
  cancelMobileMultiSelect,
  createMobileMultiSelectState,
  enterMobileMultiSelect,
  isMobileMultiSelectAllSelected,
  resetMobileMultiSelectForContext,
  selectAllMobileMultiSelectItems,
  toggleMobileMultiSelectItem,
} from "../selection/mobileMultiSelect";
import {
  buildCategoryListRequest,
  buildPhotoTimelineRequest,
  filterRecentVideosByMediaFilter,
  isMobileCategory,
  type MobileCategoryKey,
} from "./mobileCategoryActions";
import { MobileCategoryView, type PhotoCategoryMode, type VideoCategoryMode } from "./MobileCategoryView";

type MobileMoveRequest = {
  targets: DriveFile[];
  ids: string[];
};

export function MobileCategoryPage() {
  const params = useParams();

  if (!isMobileCategory(params.category)) {
    return <Navigate to="/m" replace />;
  }

  return <MobileCategoryController category={params.category} />;
}

function MobileCategoryController({ category }: { category: MobileCategoryKey }) {
  const { t } = useTranslation();
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchDraft, setSearchDraft] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [photoMode, setPhotoMode] = useState<PhotoCategoryMode>("timeline");
  const [videoMode, setVideoMode] = useState<VideoCategoryMode>("all");
  const [videoFilter, setVideoFilter] = useState<FileQueryMediaFilter>("all");
  const [documentSubtype, setDocumentSubtype] = useState<FileQueryDocumentSubtype>("all");
  const [audioSort, setAudioSort] = useState("updated_at");
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [cursor, setCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [error, setError] = useState("");
  const [recentVideos, setRecentVideos] = useState<DriveFile[]>([]);
  const [photoMonths, setPhotoMonths] = useState<PhotoMonthIndexItem[]>([]);
  const [activePhotoMonth, setActivePhotoMonth] = useState<PhotoMonthIndexItem | null>(null);
  const [timelineFiles, setTimelineFiles] = useState<DriveFile[]>([]);
  const [timelineCursor, setTimelineCursor] = useState("");
  const [timelineHasMore, setTimelineHasMore] = useState(false);
  const [timelineLoading, setTimelineLoading] = useState(false);
  const [timelineLoadingMore, setTimelineLoadingMore] = useState(false);
  const [timelineError, setTimelineError] = useState("");
  const [refreshNonce, setRefreshNonce] = useState(0);
  const [selection, setSelection] = useState(() => createMobileMultiSelectState());
  const [moveRequest, setMoveRequest] = useState<MobileMoveRequest | null>(null);
  const [moveCurrentDir, setMoveCurrentDir] = useState("/");
  const [moveDirs, setMoveDirs] = useState<DriveFile[]>([]);
  const [moveLoading, setMoveLoading] = useState(false);
  const [moveLoadingMore, setMoveLoadingMore] = useState(false);
  const [moveCursor, setMoveCursor] = useState("");
  const [moveHasMore, setMoveHasMore] = useState(false);
  const [moving, setMoving] = useState(false);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchDeleting, setBatchDeleting] = useState(false);
  const moveCurrentDirRef = useRef(moveCurrentDir);
  moveCurrentDirRef.current = moveCurrentDir;
  const shouldLoadList = category !== "photos" || photoMode === "list";
  const selectionContext = [
    "category",
    category,
    photoMode,
    videoMode,
    videoFilter,
    documentSubtype,
    audioSort,
    searchQuery,
    activePhotoMonth ? `${activePhotoMonth.year}-${activePhotoMonth.month}` : "no-month",
  ].join(":");

  useEffect(() => {
    setSearchOpen(false);
    setSearchDraft("");
    setSearchQuery("");
    setPhotoMode("timeline");
    setVideoMode("all");
    setVideoFilter("all");
    setDocumentSubtype("all");
    setAudioSort("updated_at");
    setFiles([]);
    setCursor("");
    setHasMore(false);
    setError("");
    setTimelineFiles([]);
    setTimelineCursor("");
    setTimelineHasMore(false);
    setTimelineError("");
    setSelection(createMobileMultiSelectState());
  }, [category]);

  useEffect(() => {
    setSelection((current) => resetMobileMultiSelectForContext(current, selectionContext));
  }, [selectionContext]);

  useEffect(() => {
    if (!moveRequest) return;
    let cancelled = false;
    const movingDirIds = new Set(
      moveRequest.targets.filter((file) => file.is_dir).map((file) => file.id),
    );

    setMoveLoading(true);
    listFiles(moveCurrentDir, { sort: "name", limit: 100 })
      .then((response) => {
        if (cancelled) return;
        setMoveDirs(
          (response.files ?? []).filter((file) => file.is_dir && !movingDirIds.has(file.id)),
        );
        setMoveCursor(response.next_cursor);
        setMoveHasMore(response.has_more && response.files.every((file) => file.is_dir));
      })
      .catch((err) => {
        if (cancelled) return;
        setMoveDirs([]);
        message.error(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => {
        if (!cancelled) setMoveLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [moveCurrentDir, moveRequest, t]);

  const loadMoreMoveDirs = useCallback(() => {
    if (!moveRequest || !moveCursor || !moveHasMore || moveLoadingMore) return;
    const movingDirIds = new Set(
      moveRequest.targets.filter((file) => file.is_dir).map((file) => file.id),
    );
    const path = moveCurrentDir;
    setMoveLoadingMore(true);
    listFiles(path, { sort: "name", cursor: moveCursor, limit: 100 })
      .then((response) => {
        if (moveCurrentDirRef.current !== path) return;
        const nextDirs = response.files.filter(
          (file) => file.is_dir && !movingDirIds.has(file.id),
        );
        setMoveDirs((current) => appendFolderPage(current, nextDirs));
        setMoveCursor(response.next_cursor);
        setMoveHasMore(response.has_more && response.files.every((file) => file.is_dir));
      })
      .catch((err) => message.error(err instanceof Error ? err.message : t("drive.loadError")))
      .finally(() => setMoveLoadingMore(false));
  }, [moveCurrentDir, moveCursor, moveHasMore, moveLoadingMore, moveRequest, t]);

  useEffect(() => {
    if (!shouldLoadList) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    setCursor("");
    setHasMore(false);
    queryFiles(
      buildCategoryListRequest(category, {
        query: searchQuery,
        sort: category === "audio" ? audioSort : "updated_at",
        mediaFilter: videoFilter,
        documentSubtype,
      }),
    )
      .then((response) => {
        if (cancelled) return;
        setFiles(response.items ?? []);
        setCursor(response.next_cursor ?? "");
        setHasMore(Boolean(response.has_more));
      })
      .catch((err) => {
        if (cancelled) return;
        setFiles([]);
        setError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [audioSort, category, documentSubtype, refreshNonce, searchQuery, shouldLoadList, t, videoFilter]);

  useEffect(() => {
    if (category !== "videos") return;
    let cancelled = false;
    listRecentlyViewedFiles(20)
      .then((response) => {
        if (cancelled) return;
        setRecentVideos((response.files ?? []).filter(isVideoFile));
      })
      .catch(() => {
        if (!cancelled) setRecentVideos([]);
      });
    return () => {
      cancelled = true;
    };
  }, [category, refreshNonce]);

  useEffect(() => {
    if (category !== "photos" || photoMode !== "timeline") return;
    let cancelled = false;
    listPhotoMonths()
      .then((response) => {
        if (cancelled) return;
        const months = response.months ?? [];
        setPhotoMonths(months);
        if (months.length === 0) {
          setTimelineFiles([]);
          setTimelineCursor("");
          setTimelineHasMore(false);
        }
        setActivePhotoMonth((current) => {
          if (current && months.some((item) => sameMonth(item, current))) {
            return current;
          }
          return months[0] ?? null;
        });
      })
      .catch((err) => {
        if (cancelled) return;
        setPhotoMonths([]);
        setActivePhotoMonth(null);
        setTimelineError(err instanceof Error ? err.message : t("drive.loadError"));
      });
    return () => {
      cancelled = true;
    };
  }, [category, photoMode, refreshNonce, t]);

  useEffect(() => {
    if (category !== "photos" || photoMode !== "timeline" || !activePhotoMonth) return;
    let cancelled = false;
    setTimelineLoading(true);
    setTimelineError("");
    setTimelineFiles([]);
    setTimelineCursor("");
    setTimelineHasMore(false);
    queryPhotoTimeline(buildPhotoTimelineRequest(activePhotoMonth, { query: searchQuery }))
      .then((response) => {
        if (cancelled) return;
        setTimelineFiles(response.items ?? []);
        setTimelineCursor(response.next_cursor ?? "");
        setTimelineHasMore(Boolean(response.has_more));
      })
      .catch((err) => {
        if (cancelled) return;
        setTimelineFiles([]);
        setTimelineError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => {
        if (!cancelled) setTimelineLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [
    activePhotoMonth?.month,
    activePhotoMonth?.year,
    category,
    photoMode,
    refreshNonce,
    searchQuery,
    t,
  ]);

  const loadMore = useCallback(() => {
    if (!shouldLoadList || !hasMore || loadingMore || !cursor) return;
    setLoadingMore(true);
    queryFiles(
      buildCategoryListRequest(category, {
        query: searchQuery,
        sort: category === "audio" ? audioSort : "updated_at",
        cursor,
        mediaFilter: videoFilter,
        documentSubtype,
      }),
    )
      .then((response) => {
        setFiles((current) => [...current, ...(response.items ?? [])]);
        setCursor(response.next_cursor ?? "");
        setHasMore(Boolean(response.has_more));
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => setLoadingMore(false));
  }, [
    audioSort,
    category,
    cursor,
    documentSubtype,
    hasMore,
    loadingMore,
    searchQuery,
    shouldLoadList,
    t,
    videoFilter,
  ]);

  const loadMoreTimeline = useCallback(() => {
    if (
      category !== "photos" ||
      photoMode !== "timeline" ||
      !activePhotoMonth ||
      !timelineHasMore ||
      timelineLoadingMore ||
      !timelineCursor
    ) {
      return;
    }
    setTimelineLoadingMore(true);
    queryPhotoTimeline(
      buildPhotoTimelineRequest(activePhotoMonth, {
        query: searchQuery,
        cursor: timelineCursor,
      }),
    )
      .then((response) => {
        setTimelineFiles((current) => [...current, ...(response.items ?? [])]);
        setTimelineCursor(response.next_cursor ?? "");
        setTimelineHasMore(Boolean(response.has_more));
      })
      .catch((err) => {
        setTimelineError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => setTimelineLoadingMore(false));
  }, [
    activePhotoMonth,
    category,
    photoMode,
    searchQuery,
    t,
    timelineCursor,
    timelineHasMore,
    timelineLoadingMore,
  ]);

  function submitSearch() {
    const query = searchDraft.trim();
    setSearchQuery(query);
    if (!query) {
      setSearchDraft("");
    }
  }

  function clearSearch() {
    setSearchDraft("");
    setSearchQuery("");
    setError("");
    setTimelineError("");
  }

  function cancelSearch() {
    setSearchOpen(false);
    if (!searchQuery) setSearchDraft("");
  }

  function updateSearchDraft(value: string) {
    setSearchDraft(value);
    if (!value.trim() && searchQuery) {
      setSearchQuery("");
    }
  }

  const filteredRecentVideos = filterRecentVideosByMediaFilter(recentVideos, videoFilter);
  const visibleFiles =
    category === "videos" && videoMode === "recent" ? filteredRecentVideos : files;
  const selectableFiles =
    category === "photos" && photoMode === "timeline" ? timelineFiles : visibleFiles;
  const selectedCount = selection.selectedIds.length;
  const allSelected = isMobileMultiSelectAllSelected(
    selection,
    selectableFiles.map((file) => file.id),
  );
  const moveReason = moveRequest
    ? mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir)
    : "";
  const moveDisabledText =
    moveReason === "alreadyHere"
      ? t("moveDialog.alreadyHere")
      : moveReason === "cannotMoveToSelf"
        ? t("moveDialog.cannotMoveToSelf")
        : "";

  function enterSelection(file: DriveFile) {
    setSelection((current) => enterMobileMultiSelect(current, selectionContext, file.id));
  }

  function toggleSelection(file: DriveFile) {
    setSelection((current) => toggleMobileMultiSelectItem(current, file.id));
  }

  function cancelSelection() {
    setSelection((current) => cancelMobileMultiSelect(current));
  }

  function selectAllVisibleFiles() {
    setSelection((current) =>
      selectAllMobileMultiSelectItems(
        current,
        selectionContext,
        selectableFiles.map((file) => file.id),
      ),
    );
  }

  function selectedFiles() {
    const ids = new Set(selection.selectedIds);
    return selectableFiles.filter((file) => ids.has(file.id));
  }

  function requestBatchMove() {
    const targets = selectedFiles();
    if (targets.length === 0) return;
    setMoveRequest({
      targets,
      ids: targets.map((file) => file.id),
    });
    setMoveCurrentDir("/");
    setMoveDirs([]);
  }

  function requestBatchDelete() {
    if (selection.selectedIds.length === 0) return;
    setBatchDeleteOpen(true);
  }

  function showBatchResult(result: { total: number; succeeded: number; failed: number }) {
    const feedback = mobileBatchResultFeedback(result);
    message[feedback.tone](t(feedback.key, result));
  }

  async function confirmBatchDelete() {
    const ids = [...selection.selectedIds];
    if (ids.length === 0) return;

    setBatchDeleting(true);
    try {
      const result = await batchDeleteFiles(ids);
      setBatchDeleteOpen(false);
      cancelSelection();
      setRefreshNonce((current) => current + 1);
      showBatchResult(result);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.deleteFailed"));
    } finally {
      setBatchDeleting(false);
    }
  }

  async function confirmMoveHere() {
    if (!moveRequest) return;
    const reason = mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir);
    if (reason) return;

    setMoving(true);
    try {
      const result = await batchMoveFiles(moveRequest.ids, moveCurrentDir);
      setMoveRequest(null);
      cancelSelection();
      setRefreshNonce((current) => current + 1);
      showBatchResult(result);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("moveDialog.failed"));
    } finally {
      setMoving(false);
    }
  }

  return (
    <>
      <MobileCategoryView
        category={category}
        searchOpen={searchOpen}
        searchDraft={searchDraft}
        searchActive={Boolean(searchQuery)}
        loading={loading}
        error={category === "photos" && photoMode === "timeline" ? timelineError : error}
        files={visibleFiles}
        hasMore={category === "videos" && videoMode === "recent" ? false : hasMore}
        isLoadingMore={loadingMore}
        photoMode={photoMode}
        videoMode={videoMode}
        videoFilter={videoFilter}
        documentSubtype={documentSubtype}
        audioSort={audioSort}
        recentFiles={recentVideos}
        photoMonths={photoMonths}
        activePhotoMonth={activePhotoMonth}
        timelineFiles={timelineFiles}
        timelineHasMore={timelineHasMore}
        timelineLoading={timelineLoading || timelineLoadingMore}
        selectionActive={selection.active}
        selectedIds={selection.selectedIds}
        selectedCount={selectedCount}
        allSelected={allSelected}
        onOpenSearch={() => setSearchOpen(true)}
        onCancelSearch={cancelSearch}
        onClearSearch={clearSearch}
        onSearchDraftChange={updateSearchDraft}
        onSearchSubmit={submitSearch}
        onPhotoModeChange={setPhotoMode}
        onVideoModeChange={setVideoMode}
        onVideoFilterChange={(filter) => setVideoFilter(filter as FileQueryMediaFilter)}
        onDocumentSubtypeChange={(subtype) => setDocumentSubtype(subtype as FileQueryDocumentSubtype)}
        onAudioSortChange={setAudioSort}
        onPhotoMonthSelect={setActivePhotoMonth}
        onListEndReached={loadMore}
        onTimelineEndReached={loadMoreTimeline}
        onLongPressFile={enterSelection}
        onToggleSelection={toggleSelection}
        onCancelSelection={cancelSelection}
        onSelectAll={selectAllVisibleFiles}
        onBatchMove={requestBatchMove}
        onBatchDelete={requestBatchDelete}
      />
      <MobileMoveTargetPrompt
        open={Boolean(moveRequest)}
        title={t("mobile.selection.batchMoveTitle", { count: moveRequest?.ids.length ?? 0 })}
        currentDir={moveCurrentDir}
        dirs={moveDirs}
        loading={moveLoading}
        loadingMore={moveLoadingMore}
        hasMore={moveHasMore}
        busy={moving}
        disabledReason={moveDisabledText}
        onClose={() => setMoveRequest(null)}
        onMoveHere={() => void confirmMoveHere()}
        onEnterDir={(dir) => setMoveCurrentDir(joinMobileMovePath(dir.path || "/", dir.name))}
        onGoToDir={setMoveCurrentDir}
        onLoadMore={loadMoreMoveDirs}
      />
      <MobileConfirmPrompt
        open={batchDeleteOpen}
        title={t("mobile.selection.confirmBatchDelete")}
        description={t("mobile.selection.batchDeleteBody", { count: selectedCount })}
        confirmText={t("drive.deleteToTrash")}
        tone="danger"
        busy={batchDeleting}
        onCancel={() => setBatchDeleteOpen(false)}
        onConfirm={() => void confirmBatchDelete()}
      />
    </>
  );
}

function sameMonth(a: PhotoMonthIndexItem, b: PhotoMonthIndexItem) {
  return a.year === b.year && a.month === b.month;
}

function isVideoFile(file: DriveFile) {
  const mime = file.mime_type.toLowerCase();
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  return mime.startsWith("video/") || ["mp4", "mov", "m4v", "mkv", "webm", "flv", "avi"].includes(ext);
}
