import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";
import { deleteFile, downloadUrl, listFiles, renameFile, searchFiles } from "../../api/fileApi";
import type { DriveFile, FileSearchHit } from "../../types";
import { message } from "../../components/base";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import { buildDriveSearchRequest } from "../../pages/Drive/driveSearch";
import {
  canSubmitDriveRename,
  driveRenameErrorKey,
  driveRenamePayloadName,
} from "../../pages/Drive/driveRename";
import { UploadFab } from "../components/UploadFab/UploadFab";
import { mobilePathFromSearch } from "../utils/mobilePath";
import { MobileDriveView } from "./MobileDriveView";
import { startMobileDriveUploads } from "./mobileUpload";
import styles from "./MobilePlaceholder.module.css";

export function MobileDrivePage() {
  const { t } = useTranslation();
  const location = useLocation();
  const currentPath = mobilePathFromSearch(location.search);
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
  const { upload } = useChunkedUpload(() => {
    void refresh();
  });

  function refresh() {
    let cancelled = false;
    setLoading(true);
    listFiles(currentPath)
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
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }

  useEffect(() => {
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
  }

  async function handleUploadFiles(selected: FileList | null) {
    const count = await startMobileDriveUploads(selected, currentPath, upload);
    if (count > 0) {
      message.info(t("drive.filesAddedToTransfer", { count }));
    }
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
        onOpenActions={setActionFile}
        onCloseActions={() => setActionFile(null)}
        onSearchDraftChange={setSearchDraft}
        onSearchSubmit={() => void handleSearchSubmit()}
        onClearSearch={clearSearch}
        onSemanticChange={setIncludeSemantic}
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
      />
      <UploadFab onFiles={(selected) => void handleUploadFiles(selected)} />
    </section>
  );
}
