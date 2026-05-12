import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import type { DriveFile, FileSearchHit } from "../../types";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import { MobileFileCard } from "../components/FileCard/MobileFileCard";
import { MobileTextPrompt } from "../components/TextPrompt/MobileTextPrompt";
import { mobileFilesHref, mobilePreviewHref, normalizeMobilePath } from "../utils/mobilePath";
import styles from "./MobileDriveView.module.css";

interface MobileDriveViewProps {
  currentPath: string;
  files: DriveFile[];
  loading?: boolean;
  error?: string;
  searchDraft?: string;
  searchActive?: boolean;
  searchHits?: FileSearchHit[];
  searching?: boolean;
  searchError?: string;
  includeSemantic?: boolean;
  actionFile?: DriveFile | null;
  actionDownloadHref?: string;
  deleteConfirmFile?: DriveFile | null;
  deleteConfirmBusy?: boolean;
  renameFile?: DriveFile | null;
  renameDraft?: string;
  renameError?: string;
  renameBusy?: boolean;
  createFolderOpen?: boolean;
  createFolderDraft?: string;
  createFolderBusy?: boolean;
  onSearchDraftChange?: (query: string) => void;
  onSearchSubmit?: () => void;
  onClearSearch?: () => void;
  onSemanticChange?: (includeSemantic: boolean) => void;
  onOpenCreateFolder?: () => void;
  onCreateFolderDraftChange?: (value: string) => void;
  onCancelCreateFolder?: () => void;
  onConfirmCreateFolder?: () => void;
  onOpenActions?: (file: DriveFile) => void;
  onCloseActions?: () => void;
  onRename?: (file: DriveFile) => void;
  onDelete?: (file: DriveFile) => void;
  onCancelDelete?: () => void;
  onConfirmDelete?: () => void;
  onRenameDraftChange?: (value: string) => void;
  onCancelRename?: () => void;
  onConfirmRename?: () => void;
}

export function MobileDriveView({
  currentPath,
  files,
  loading = false,
  error = "",
  searchDraft = "",
  searchActive = false,
  searchHits = [],
  searching = false,
  searchError = "",
  includeSemantic = false,
  actionFile = null,
  actionDownloadHref = "",
  deleteConfirmFile = null,
  deleteConfirmBusy = false,
  renameFile = null,
  renameDraft = "",
  renameError = "",
  renameBusy = false,
  createFolderOpen = false,
  createFolderDraft = "",
  createFolderBusy = false,
  onSearchDraftChange,
  onSearchSubmit,
  onClearSearch,
  onSemanticChange,
  onOpenCreateFolder,
  onCreateFolderDraftChange,
  onCancelCreateFolder,
  onConfirmCreateFolder,
  onOpenActions,
  onCloseActions,
  onRename,
  onDelete,
  onCancelDelete,
  onConfirmDelete,
  onRenameDraftChange,
  onCancelRename,
  onConfirmRename,
}: MobileDriveViewProps) {
  const { t } = useTranslation();

  return (
    <>
      <form
        className={styles.searchBar}
        role="search"
        onSubmit={(event) => {
          event.preventDefault();
          onSearchSubmit?.();
        }}
      >
        <span className="material-symbols-outlined" aria-hidden>
          search
        </span>
        <input
          value={searchDraft}
          placeholder={t("drive.searchPlaceholder")}
          onChange={(event) => onSearchDraftChange?.(event.target.value)}
        />
        {searchActive ? (
          <button type="button" onClick={onClearSearch}>
            {t("searchResultList.clearSearch")}
          </button>
        ) : (
          <button type="submit">{t("common.search")}</button>
        )}
      </form>
      <button
        className={`${styles.semanticToggle} ${includeSemantic ? styles.semanticToggleActive : ""}`}
        type="button"
        role="switch"
        aria-checked={includeSemantic}
        onClick={() => onSemanticChange?.(!includeSemantic)}
      >
        <span className="material-symbols-outlined" aria-hidden>
          psychology
        </span>
        <span>
          <strong>{t("mobile.files.semanticSearch")}</strong>
          <small>{t("mobile.files.semanticSearchHint")}</small>
        </span>
      </button>
      <div className={styles.quickActions}>
        <button type="button" onClick={() => onOpenCreateFolder?.()}>
          <span className="material-symbols-outlined" aria-hidden>
            create_new_folder
          </span>
          {t("drive.newFolder")}
        </button>
      </div>

      {searchActive ? (
        <SearchResults
          currentPath={currentPath}
          hits={searchHits}
          loading={searching}
          error={searchError}
          onOpenActions={onOpenActions}
        />
      ) : loading && files.length === 0 ? (
        <div className={styles.state}>{t("common.loading")}</div>
      ) : error ? (
        <div className={styles.state}>{error}</div>
      ) : files.length === 0 ? (
        <div className={styles.state}>{t("mobile.files.empty")}</div>
      ) : (
        <div className={styles.list}>
          {files.map((file) => (
            <MobileFileCard
              key={file.id}
              file={file}
              currentPath={currentPath}
              onMore={onOpenActions}
            />
          ))}
        </div>
      )}
      {actionFile ? (
        <section
          className={styles.actionSheet}
          role="dialog"
          aria-modal="true"
          aria-label={t("mobile.files.actionsTitle", { name: actionFile.name })}
        >
          <button className={styles.sheetBackdrop} type="button" onClick={onCloseActions} />
          <div className={styles.sheetPanel}>
            <div className={styles.sheetHandle} />
            <header className={styles.sheetHeader}>
              <strong>{actionFile.name}</strong>
              <button type="button" onClick={onCloseActions} aria-label={t("common.close")}>
                <span className="material-symbols-outlined" aria-hidden>
                  close
                </span>
              </button>
            </header>
            <div className={styles.sheetActions}>
              {!actionFile.is_dir && actionDownloadHref ? (
                <a href={actionDownloadHref} download={actionFile.name}>
                  <span className="material-symbols-outlined" aria-hidden>
                    download
                  </span>
                  {t("common.download")}
                </a>
              ) : null}
              <button type="button" onClick={() => onRename?.(actionFile)}>
                <span className="material-symbols-outlined" aria-hidden>
                  edit
                </span>
                {t("common.rename")}
              </button>
              <button
                className={styles.dangerAction}
                type="button"
                onClick={() => onDelete?.(actionFile)}
              >
                <span className="material-symbols-outlined" aria-hidden>
                  delete
                </span>
                {t("drive.deleteToTrash")}
              </button>
            </div>
          </div>
        </section>
      ) : null}
      <MobileConfirmPrompt
        open={Boolean(deleteConfirmFile)}
        title={t("drive.confirmDelete")}
        description={
          deleteConfirmFile
            ? t("drive.deleteConfirmBody", { name: deleteConfirmFile.name })
            : ""
        }
        confirmText={t("drive.deleteToTrash")}
        tone="danger"
        busy={deleteConfirmBusy}
        onCancel={() => onCancelDelete?.()}
        onConfirm={() => onConfirmDelete?.()}
      />
      <MobileTextPrompt
        open={Boolean(renameFile)}
        title={t("common.rename")}
        label={t("mobile.files.renameField")}
        value={renameDraft}
        error={renameError}
        confirmText={t("common.save")}
        busy={renameBusy}
        disabled={Boolean(renameError) || !renameDraft.trim() || renameDraft.trim() === renameFile?.name}
        onValueChange={(value) => onRenameDraftChange?.(value)}
        onCancel={() => onCancelRename?.()}
        onConfirm={() => onConfirmRename?.()}
      />
      <MobileTextPrompt
        open={createFolderOpen}
        title={t("drive.newFolder")}
        label={t("drive.folderName")}
        value={createFolderDraft}
        confirmText={t("common.create")}
        busy={createFolderBusy}
        disabled={!createFolderDraft.trim()}
        onValueChange={(value) => onCreateFolderDraftChange?.(value)}
        onCancel={() => onCancelCreateFolder?.()}
        onConfirm={() => onConfirmCreateFolder?.()}
      />
    </>
  );
}

function SearchResults({
  currentPath,
  hits,
  loading,
  error,
  onOpenActions,
}: {
  currentPath: string;
  hits: FileSearchHit[];
  loading: boolean;
  error: string;
  onOpenActions?: (file: DriveFile) => void;
}) {
  const { t } = useTranslation();

  if (loading && hits.length === 0) {
    return <div className={styles.state}>{t("searchResultList.searching")}</div>;
  }

  if (error) {
    return <div className={styles.state}>{error}</div>;
  }

  if (hits.length === 0) {
    return <div className={styles.state}>{t("searchResultList.noResults")}</div>;
  }

  return (
    <section className={styles.searchResults}>
      <div className={styles.searchInfo}>
        <strong>{t("searchResultList.foundResults", { count: hits.length })}</strong>
      </div>
      <div className={styles.list}>
        {hits.map((hit) => (
          <article key={`${hit.file.id}-${hit.score}`} className={styles.searchCard}>
            <Link className={styles.searchMain} to={mobileSearchHref(hit.file, currentPath)}>
              <span className={styles.searchIcon}>
                <span className="material-symbols-outlined" aria-hidden>
                  {hit.file.is_dir ? "folder" : "description"}
                </span>
              </span>
              <span className={styles.searchText}>
                <strong>{hit.file.name}</strong>
                {hit.snippet ? <span>{hit.snippet}</span> : null}
              </span>
              <span className={styles.searchScore}>{Math.round(hit.score * 100)}%</span>
            </Link>
            <button
              className={styles.searchMore}
              type="button"
              aria-label={t("mobile.files.moreActions", { name: hit.file.name })}
              onClick={() => onOpenActions?.(hit.file)}
            >
              <span className="material-symbols-outlined" aria-hidden>
                more_vert
              </span>
            </button>
          </article>
        ))}
      </div>
    </section>
  );
}

function mobileSearchHref(file: DriveFile, fallbackPath: string) {
  const parentPath = normalizeMobilePath(file.path || fallbackPath);
  if (file.is_dir) {
    return mobileFilesHref(
      normalizeMobilePath(`${parentPath === "/" ? "" : parentPath}/${file.name}`),
    );
  }
  return mobilePreviewHref(file.id, parentPath);
}
