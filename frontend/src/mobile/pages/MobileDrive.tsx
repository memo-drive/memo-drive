import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";
import {
  batchDeleteFiles,
  batchMoveFiles,
  createFolder,
  deleteFile,
  downloadUrl,
  listFiles,
  moveFile,
  renameFile,
  searchFiles,
} from "../../api/fileApi";
import type { DriveFile, FileSearchHit } from "../../types";
import { message } from "../../components/base";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import {
  buildDriveSearchRequest,
  canSubmitDriveFolder,
  canSubmitDriveRename,
  completeDriveFolderCreate,
  driveFolderPayloadName,
  driveRenameErrorKey,
  driveRenamePayloadName,
  startDriveFolderEntry,
  startDriveFolderCreate,
} from "../../workflows/driveWorkflow";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import { UploadFab } from "../components/UploadFab/UploadFab";
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
import { startMobileDriveUploads } from "./mobileUpload";
import styles from "./MobilePlaceholder.module.css";

type MobileMoveRequest = {
  kind: "single" | "batch";
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
  const [error, setError] = useState("");
  const [searchDraft, setSearchDraft] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const [searchHits, setSearchHits] = useState<FileSearchHit[]>([]);
  const [searching, setSearching] = useState(false);
  const [searchError, setSearchError] = useState("");
  const [includeSemantic, setIncludeSemantic] = useState(false);
  const [actionFile, setActionFile] = useState<DriveFile | null>(null);
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
  const [moving, setMoving] = useState(false);
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [batchDeleting, setBatchDeleting] = useState(false);
  const enteringFolderIdRef = useRef<string | null>(null);
  const enteringPathRef = useRef<string | null>(null);
  const { upload } = useChunkedUpload(() => {
    void refresh();
  });

  function refresh() {
    let cancelled = false;
    const path = currentPath;
    setLoading(true);
    listFiles(path)
      .then((response) => {
        if (cancelled) return;
        setFiles(response.files ?? []);
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

  useEffect(() => {
    setSelection((current) => resetMobileMultiSelectForContext(current, selectionContext));
  }, [selectionContext]);

  useEffect(() => {
    if (!setBottomNavHidden) return;
    setBottomNavHidden(selection.active);
    return () => {
      setBottomNavHidden(false);
    };
  }, [selection.active, setBottomNavHidden]);

  useEffect(() => {
    if (!moveRequest) return;
    let cancelled = false;
    const movingDirIds = new Set(
      moveRequest.targets.filter((file) => file.is_dir).map((file) => file.id),
    );

    setMoveLoading(true);
    listFiles(moveCurrentDir)
      .then((response) => {
        if (cancelled) return;
        setMoveDirs(
          (response.files ?? []).filter((file) => file.is_dir && !movingDirIds.has(file.id)),
        );
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

  function handleUploadFiles(selected: FileList | null) {
    const count = startMobileDriveUploads(selected, currentPath, upload, (file, err) => {
      if (err instanceof Error && err.message === "upload cancelled") return;
      message.error(t("drive.uploadFailed", { name: file.name }));
    });
    if (count > 0) {
      message.info(t("drive.filesAddedToTransfer", { count }));
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
    const reason = mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir);
    if (reason) return;

    setMoving(true);
    try {
      if (moveRequest.kind === "single") {
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
      message.error(err instanceof Error ? err.message : t("moveDialog.failed"));
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
    ? mobileMoveDisabledReason(moveRequest.targets, moveCurrentDir)
    : "";
  const moveTitle = moveRequest
    ? moveRequest.kind === "single"
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
        enteringFolderId={enteringFolderId}
        error={error}
        searchDraft={searchDraft}
        searchActive={Boolean(searchQuery)}
        searchHits={searchHits}
        searching={searching}
        searchError={searchError}
        includeSemantic={includeSemantic}
        actionFile={actionFile}
        actionDownloadHref={actionFile && !actionFile.is_dir ? downloadUrl(actionFile.id) : ""}
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
      />
      {selection.active ? null : <UploadFab onFiles={handleUploadFiles} />}
      <MobileMoveTargetPrompt
        open={Boolean(moveRequest)}
        title={moveTitle}
        currentDir={moveCurrentDir}
        dirs={moveDirs}
        loading={moveLoading}
        busy={moving}
        disabledReason={moveDisabledText}
        onClose={() => setMoveRequest(null)}
        onMoveHere={() => void confirmMoveHere()}
        onEnterDir={(dir) => setMoveCurrentDir(joinMobileMovePath(dir.path || "/", dir.name))}
        onGoToDir={setMoveCurrentDir}
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
    </section>
  );
}
