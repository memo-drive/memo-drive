import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";
import {
  batchDeleteFiles,
  batchMoveFiles,
  createFolder,
	copyFile,
  deleteFile,
  downloadUrl,
	folderZIPDownloadUrl,
  listFiles,
  moveFile,
  renameFile,
  searchFiles,
} from "../../api/fileApi";
import { prepareDirectoryUpload } from "../../api/uploadApi";
import type { DriveFile, FileSearchHit } from "../../types";
import { message } from "../../components/base";
import { VersionHistoryDialog } from "../../components/FileVersion/VersionHistoryDialog";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import { useUploadConflictResolver } from "../../hooks/useUploadConflictResolver";
import {
  buildDriveSearchRequest,
  canSubmitDriveFolder,
  canSubmitDriveRename,
  completeDriveFolderCreate,
  driveFolderPayloadName,
  driveRenameErrorKey,
  driveRenamePayloadName,
  selectedDriveUploadFiles,
  startDriveFolderEntry,
  startDriveFolderCreate,
	startDriveVersionHistory,
} from "../../workflows/driveWorkflow";
import { appendFolderPage } from "../../workflows/folderPagination";
import { buildCopyRequest } from "../../workflows/copyWorkflow";
import {
  matchPreparedDirectoryEntries,
  selectedDirectoryUploadEntries,
} from "../../workflows/directoryUploadWorkflow";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import { UploadFab } from "../components/UploadFab/UploadFab";
import { MobileUploadConflictSheet } from "../components/UploadConflictSheet/MobileUploadConflictSheet";
import { useMobileShellChrome } from "../layouts/MobileShell";
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
import { mobilePathFromSearch } from "../utils/mobilePath";
import { MobileDriveView } from "./MobileDriveView";
import styles from "./MobilePlaceholder.module.css";

type MobileMoveRequest = {
	kind: "single" | "batch" | "copy";
  targets: DriveFile[];
  ids: string[];
};

export function MobileDrivePage() {
  const { t } = useTranslation();
  const location = useLocation();
  const currentPath = mobilePathFromSearch(location.search);
  const shellChrome = useMobileShellChrome();
  const setBottomNavHidden = shellChrome?.setBottomNavHidden;
  const selectionContext = `files:${currentPath}`;
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searchHits, setSearchHits] = useState<FileSearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [includeSemantic, setIncludeSemantic] = useState(false);
  const [actionFile, setActionFile] = useState<DriveFile | null>(null);
	const [versionHistoryFile, setVersionHistoryFile] = useState<DriveFile | null>(null);
  const [deleteConfirmFile, setDeleteConfirmFile] = useState<DriveFile | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [renameTarget, setRenameTarget] = useState<DriveFile | null>(null);
  const [renameDraft, setRenameDraft] = useState("");
  const [renaming, setRenaming] = useState(false);
  const [createFolderOpen, setCreateFolderOpen] = useState(false);
  const [createFolderDraft, setCreateFolderDraft] = useState("");
  const [creatingFolder, setCreatingFolder] = useState(false);
  const [enteringFolderId, setEnteringFolderId] = useState<string | null>(null);
  const [selection, setSelection] = useState(() =>
    createMobileMultiSelectState(selectionContext),
  );
  const [moveRequest, setMoveRequest] = useState<MobileMoveRequest | null>(null);
  const [moveCurrentDir, setMoveCurrentDir] = useState("/");
  const [moveDirs, setMoveDirs] = useState<DriveFile[]>([]);
  const [moveLoading, setMoveLoading] = useState(false);
  const [moveLoadingMore, setMoveLoadingMore] = useState(false);
  const [moveNextCursor, setMoveNextCursor] = useState("");
  const [moveHasMore, setMoveHasMore] = useState(false);
  const [moving, setMoving] = useState(false);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchDeleting, setBatchDeleting] = useState(false);
  const enteringFolderIdRef = useRef<string | null>(null);
  const enteringPathRef = useRef<string | null>(null);
  const currentPathRef = useRef(currentPath);
  const moveCurrentDirRef = useRef(moveCurrentDir);
  currentPathRef.current = currentPath;
  moveCurrentDirRef.current = moveCurrentDir;
  const { upload } = useChunkedUpload(() => {
    void refresh();
  });
  const uploadConflicts = useUploadConflictResolver();

  function refresh() {
    let cancelled = false;
    const path = currentPath;
    setLoading(true);
    listFiles(path, { limit: 100 })
      .then((response) => {
        if (cancelled) return;
        setFiles(response.files ?? []);
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
        setError("");
      })
      .catch((err) => {
        if (cancelled) return;
        setFiles([]);
        setError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => {
        if (cancelled) return;
        setLoading(false);
        if (enteringPathRef.current === path) {
          enteringPathRef.current = null;
          enteringFolderIdRef.current = null;
          setEnteringFolderId(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }

  const loadMore = useCallback(() => {
    if (!hasMore || !nextCursor || loadingMore) return;
    const path = currentPath;
    setLoadingMore(true);
    listFiles(path, { cursor: nextCursor, limit: 100 })
      .then((response) => {
        if (currentPathRef.current !== path) return;
        setFiles((current) => appendFolderPage(current, response.files ?? []));
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : t("drive.loadError"));
      })
      .finally(() => setLoadingMore(false));
  }, [currentPath, hasMore, loadingMore, nextCursor, t]);

  useEffect(() => {
    setSelection((current) => resetMobileMultiSelectForContext(current, selectionContext));
  }, [selectionContext]);

  useEffect(() => {
    if (!setBottomNavHidden) return;
    setBottomNavHidden(selection.active || Boolean(uploadConflicts.conflict));
    return () => {
      setBottomNavHidden(false);
    };
  }, [selection.active, setBottomNavHidden, uploadConflicts.conflict]);

  useEffect(() => {
    if (!moveRequest) return;
    let cancelled = false;
	const movingDirIds = new Set(
	  moveRequest.kind === "copy" ? [] : moveRequest.targets.filter((file) => file.is_dir).map((file) => file.id),
	);

    setMoveLoading(true);
    listFiles(moveCurrentDir, { sort: "name", limit: 100 })
      .then((response) => {
        if (cancelled) return;
        setMoveDirs(
          (response.files ?? []).filter((file) => file.is_dir && !movingDirIds.has(file.id)),
        );
        setMoveNextCursor(response.next_cursor);
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
    if (!moveRequest || !moveNextCursor || !moveHasMore || moveLoadingMore) return;
	const movingDirIds = new Set(
	  moveRequest.kind === "copy" ? [] : moveRequest.targets.filter((file) => file.is_dir).map((file) => file.id),
	);
    const path = moveCurrentDir;
    setMoveLoadingMore(true);
    listFiles(path, { sort: "name", cursor: moveNextCursor, limit: 100 })
      .then((response) => {
        if (moveCurrentDirRef.current !== path) return;
        const nextDirs = response.files.filter(
          (file) => file.is_dir && !movingDirIds.has(file.id),
        );
        setMoveDirs((current) => appendFolderPage(current, nextDirs));
        setMoveNextCursor(response.next_cursor);
        setMoveHasMore(response.has_more && response.files.every((file) => file.is_dir));
      })
      .catch((err) => message.error(err instanceof Error ? err.message : t("drive.loadError")))
      .finally(() => setMoveLoadingMore(false));
  }, [moveCurrentDir, moveHasMore, moveLoadingMore, moveNextCursor, moveRequest, t]);

  useEffect(() => {
    if (enteringPathRef.current && enteringPathRef.current !== currentPath) {
      enteringPathRef.current = null;
      enteringFolderIdRef.current = null;
      setEnteringFolderId(null);
    }
    const cancel = refresh();
    setSearchDraft("");
    setSearchQuery("");
    setSearchHits([]);
    setSearchError("");
    return () => {
      cancel();
    };
  }, [currentPath, t]);

  async function handleSearchSubmit() {
    const request = buildDriveSearchRequest(searchDraft, currentPath, includeSemantic);
    if (!request) return;
    setSearching(true);
    setSearchQuery(request.query);
    setSearchError("");
    try {
      const response = await searchFiles(request);
      setSearchHits(response.hits ?? []);
    } catch (err) {
      setSearchHits([]);
      setSearchError(err instanceof Error ? err.message : t("drive.searchFailed"));
    } finally {
      setSearching(false);
    }
  }

  function clearSearch() {
    setSearchDraft("");
    setSearchQuery("");
    setSearchHits([]);
    setSearchError("");
    setSearching(false);
  }

  function handleSearchDraftChange(value: string) {
    setSearchDraft(value);
    if (!value.trim()) {
      setSearchQuery("");
      setSearchHits([]);
      setSearchError("");
      setSearching(false);
    }
  }

  function openCreateFolder() {
    const draft = startDriveFolderCreate();
    setCreateFolderOpen(draft.open);
    setCreateFolderDraft(draft.draftName);
  }

  function closeCreateFolder() {
    const draft = completeDriveFolderCreate();
    setCreateFolderOpen(draft.open);
    setCreateFolderDraft(draft.draftName);
  }

  async function confirmCreateFolder() {
    if (!canSubmitDriveFolder(createFolderDraft)) return;

    setCreatingFolder(true);
    try {
      await createFolder(currentPath, driveFolderPayloadName(createFolderDraft));
      closeCreateFolder();
      refresh();
      message.success(t("drive.createSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.createFailed"));
    } finally {
      setCreatingFolder(false);
    }
  }

  async function handleUploadFiles(selected: FileList | null) {
    const files = selectedDriveUploadFiles(selected);
    if (files.length === 0) return;
    try {
      const batch = await uploadConflicts.resolve(files, currentPath);
      if (batch.uploads.length > 0) {
        message.info(t("drive.filesAddedToTransfer", { count: batch.uploads.length }));
      }
      if (batch.skipped > 0) {
        message.info(t("uploadConflict.skippedSummary", { count: batch.skipped }));
      }
      for (const item of batch.uploads) {
        void upload(item.file, currentPath, item.overwritePolicy).catch((err) => {
          if (err instanceof Error && err.message === "upload cancelled") return;
          message.error(t("drive.uploadFailed", { name: item.file.name }));
        });
      }
    } catch (err) {
      message.error(
        err instanceof Error ? err.message : t("uploadConflict.preflightFailed"),
      );
    }
  }

  async function handleDirectoryFiles(selected: FileList | null) {
    if (!selected || selected.length === 0) return;
    try {
      const localEntries = selectedDirectoryUploadEntries(selected);
      const response = await prepareDirectoryUpload(currentPath, localEntries);
      const prepared = matchPreparedDirectoryEntries(localEntries, response);
      const batch = await uploadConflicts.resolvePrepared(
        prepared.batchId,
        prepared.uploads,
      );
      if (batch.uploads.length > 0) {
        message.info(t("drive.filesAddedToTransfer", { count: batch.uploads.length }));
      }
      const skipped = batch.skipped + prepared.failures.length;
      if (skipped > 0) {
        message.info(t("uploadConflict.skippedSummary", { count: skipped }));
      }
      for (const item of batch.uploads) {
        if (!item.destPath || !item.batchId || !item.relativePath) continue;
        void upload(item.file, item.destPath, item.overwritePolicy, {
          batchId: item.batchId,
          relativePath: item.relativePath,
        }).catch((err) => {
          if (err instanceof Error && err.message === "upload cancelled") return;
          message.error(t("drive.uploadFailed", { name: item.file.name }));
        });
      }
    } catch (err) {
      message.error(
        err instanceof Error ? err.message : t("uploadConflict.preflightFailed"),
      );
    }
  }

  function handleEnterFolder(file: DriveFile) {
    const entry = startDriveFolderEntry(file, enteringFolderIdRef.current);
    if (!entry) return false;
    enteringFolderIdRef.current = entry.enteringFolderId;
    enteringPathRef.current = entry.nextPath;
    setEnteringFolderId(entry.enteringFolderId);
    return true;
  }

  function enterSelection(file: DriveFile) {
    setActionFile(null);
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
        files.map((file) => file.id),
      ),
    );
  }

  function selectedFiles() {
    const ids = new Set(selection.selectedIds);
    return files.filter((file) => ids.has(file.id));
  }

  function requestRename(file: DriveFile) {
    setActionFile(null);
    setRenameTarget(file);
    setRenameDraft(file.name);
  }

  async function confirmRename() {
    if (!canSubmitDriveRename(renameTarget, renameDraft)) return;

    try {
      setRenaming(true);
      await renameFile(renameTarget!.id, driveRenamePayloadName(renameDraft));
      setRenameTarget(null);
      setRenameDraft("");
      refresh();
      message.success(t("drive.renameSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.renameFailed"));
    } finally {
      setRenaming(false);
    }
  }

  function requestDelete(file: DriveFile) {
    setActionFile(null);
    setDeleteConfirmFile(file);
  }

  async function confirmDelete() {
    if (!deleteConfirmFile) return;
    setDeleting(true);
    try {
      await deleteFile(deleteConfirmFile.id);
      const deletedName = deleteConfirmFile.name;
      setDeleteConfirmFile(null);
      refresh();
      message.success(t("drive.deleteSuccess", { name: deletedName }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.deleteFailed"));
    } finally {
      setDeleting(false);
    }
  }

  function openMoveRequest(request: MobileMoveRequest) {
    if (request.ids.length === 0) return;
    setActionFile(null);
    setMoveRequest(request);
    setMoveCurrentDir("/");
    setMoveDirs([]);
  }

  function requestMove(file: DriveFile) {
    openMoveRequest({ kind: "single", targets: [file], ids: [file.id] });
  }

	function requestCopy(file: DriveFile) {
		openMoveRequest({ kind: "copy", targets: [file], ids: [file.id] });
	}

  function requestBatchMove() {
    const targets = selectedFiles();
    openMoveRequest({
      kind: "batch",
      targets,
      ids: targets.map((file) => file.id),
    });
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
      refresh();
      showBatchResult(result);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.deleteFailed"));
    } finally {
      setBatchDeleting(false);
    }
  }

  async function confirmMoveHere() {
    if (!moveRequest) return;
	const reason = moveRequest.kind === "copy" ? "" : mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir);
    if (reason) return;

    setMoving(true);
    try {
	  if (moveRequest.kind === "copy") {
		const request = buildCopyRequest(moveRequest.targets[0], moveCurrentDir);
		await copyFile(request.id, request.input);
		message.success(t("copyDialog.success"));
	  } else if (moveRequest.kind === "single") {
        await moveFile(moveRequest.ids[0], moveCurrentDir);
        message.success(t("moveDialog.success"));
      } else {
        const result = await batchMoveFiles(moveRequest.ids, moveCurrentDir);
        cancelSelection();
        showBatchResult(result);
      }
      setMoveRequest(null);
      refresh();
    } catch (err) {
	  message.error(err instanceof Error ? err.message : t(moveRequest.kind === "copy" ? "copyDialog.failed" : "moveDialog.failed"));
    } finally {
      setMoving(false);
    }
  }

  const selectedCount = selection.selectedIds.length;
  const allSelected = isMobileMultiSelectAllSelected(
    selection,
    files.map((file) => file.id),
  );
  const moveReason = moveRequest
	? moveRequest.kind === "copy" ? "" : mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir)
    : "";
  const moveTitle = moveRequest
	? moveRequest.kind === "copy"
	  ? t("copyDialog.title", { name: moveRequest.targets[0]?.name ?? "" })
	  : moveRequest.kind === "single"
      ? t("mobile.selection.singleMoveTitle", { name: moveRequest.targets[0]?.name ?? "" })
      : t("mobile.selection.batchMoveTitle", { count: moveRequest.ids.length })
    : "";
  const moveDisabledText =
    moveReason === "alreadyHere"
      ? t("moveDialog.alreadyHere")
      : moveReason === "cannotMoveToSelf"
        ? t("moveDialog.cannotMoveToSelf")
        : "";

  return (
    <section className={styles.page} data-mobile-page="files">
      <h1>{t("mobile.files.title")}</h1>
      <p>{t("mobile.files.subtitle")}</p>
      <div className={styles.pathBar}>
        <span>{t("mobile.files.currentFolder")}</span>
        <strong>{currentPath}</strong>
      </div>
      <MobileDriveView
        currentPath={currentPath}
        files={files}
        loading={loading}
        loadingMore={loadingMore}
        hasMore={hasMore}
        enteringFolderId={enteringFolderId}
        error={error}
        searchDraft={searchDraft}
        searchActive={Boolean(searchQuery)}
        searchHits={searchHits}
        searching={searching}
        searchError={searchError}
        includeSemantic={includeSemantic}
        actionFile={actionFile}
		actionDownloadHref={actionFile ? (actionFile.is_dir ? folderZIPDownloadUrl(actionFile.id) : downloadUrl(actionFile.id)) : ""}
        deleteConfirmFile={deleteConfirmFile}
        deleteConfirmBusy={deleting}
        renameFile={renameTarget}
        renameDraft={renameDraft}
        renameError={driveRenameErrorKey(renameDraft) ? t(driveRenameErrorKey(renameDraft)!) : ""}
        renameBusy={renaming}
        createFolderOpen={createFolderOpen}
        createFolderDraft={createFolderDraft}
        createFolderBusy={creatingFolder}
        selectionActive={selection.active}
        selectedIds={selection.selectedIds}
        selectedCount={selectedCount}
        allSelected={allSelected}
        onOpenActions={setActionFile}
        onCloseActions={() => setActionFile(null)}
        onEnterFolder={handleEnterFolder}
        onSearchDraftChange={handleSearchDraftChange}
        onSearchSubmit={() => void handleSearchSubmit()}
        onClearSearch={clearSearch}
        onSemanticChange={setIncludeSemantic}
        onOpenCreateFolder={openCreateFolder}
        onCreateFolderDraftChange={setCreateFolderDraft}
        onCancelCreateFolder={closeCreateFolder}
        onConfirmCreateFolder={() => void confirmCreateFolder()}
		onMove={requestMove}
		onCopy={requestCopy}
		onVersionHistory={(file) => {
			setActionFile(null);
			setVersionHistoryFile(startDriveVersionHistory(file)?.target ?? null);
		}}
        onRename={requestRename}
        onDelete={requestDelete}
        onCancelDelete={() => setDeleteConfirmFile(null)}
        onConfirmDelete={() => void confirmDelete()}
        onRenameDraftChange={setRenameDraft}
        onCancelRename={() => {
          setRenameTarget(null);
          setRenameDraft("");
        }}
        onConfirmRename={() => void confirmRename()}
        onLongPressFile={enterSelection}
        onToggleSelection={toggleSelection}
        onCancelSelection={cancelSelection}
        onSelectAll={selectAllVisibleFiles}
        onBatchMove={requestBatchMove}
        onBatchDelete={requestBatchDelete}
        onEndReached={loadMore}
      />
      {selection.active ? null : (
        <UploadFab
          onFiles={(files) => void handleUploadFiles(files)}
          onDirectoryFiles={(files) => void handleDirectoryFiles(files)}
        />
      )}
      <MobileUploadConflictSheet
        conflict={uploadConflicts.conflict}
        onDecision={uploadConflicts.decide}
      />
      <MobileMoveTargetPrompt
        open={Boolean(moveRequest)}
        title={moveTitle}
		mode={moveRequest?.kind === "copy" ? "copy" : "move"}
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
	  <VersionHistoryDialog
		open={Boolean(versionHistoryFile)}
		file={versionHistoryFile}
		onClose={() => setVersionHistoryFile(null)}
		onRestored={(file) => {
			setVersionHistoryFile(file);
			refresh();
		}}
	  />
    </section>
  );
}
