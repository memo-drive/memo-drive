import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../../types";
import { filePresentation, fileSizeLabel } from "../../../components/FileManager/filePresentation";
import { LazyThumbnail } from "../../../components/Virtualized";
import { driveFolderPath } from "../../../workflows/driveWorkflow";
import { useMobileLongPress } from "../../selection/useMobileLongPress";
import { mobileFilesHref, mobilePreviewHref } from "../../utils/mobilePath";
import styles from "./MobileFileCard.module.css";

interface MobileFileCardProps {
  file: DriveFile;
  currentPath: string;
  entering?: boolean;
  folderNavigationDisabled?: boolean;
  selectionMode?: boolean;
  selected?: boolean;
  onFolderEnter?: (file: DriveFile) => boolean | void;
  onMore?: (file: DriveFile) => void;
  onSelectionToggle?: (file: DriveFile) => void;
  onLongPress?: (file: DriveFile) => void;
}

export function MobileFileCard({
  file,
  currentPath,
  entering = false,
  folderNavigationDisabled = false,
  selectionMode = false,
  selected = false,
  onFolderEnter,
  onMore,
  onSelectionToggle,
  onLongPress,
}: MobileFileCardProps) {
  const { t } = useTranslation();
  const presentation = filePresentation(file);
  const href = file.is_dir
    ? mobileFilesHref(driveFolderPath(file))
    : mobilePreviewHref(file.id, currentPath);
  const viewBusy = entering || folderNavigationDisabled;
  const longPress = useMobileLongPress(onLongPress ? () => onLongPress(file) : undefined);

  return (
    <article
      className={`${styles.card} ${selectionMode ? styles.cardSelectable : ""} ${selected ? styles.cardSelected : ""}`}
      data-mobile-selectable={selectionMode || undefined}
      aria-selected={selectionMode ? selected : undefined}
      onPointerDown={longPress.onPointerDown}
      onPointerUp={longPress.onPointerUp}
      onPointerLeave={longPress.onPointerLeave}
      onPointerCancel={longPress.onPointerCancel}
      onContextMenu={longPress.onContextMenu}
    >
      <Link
        className={`${styles.main} ${viewBusy ? styles.mainDisabled : ""}`}
        to={selectionMode ? "#" : href}
        aria-disabled={viewBusy || undefined}
        onClick={(event) => {
          if (longPress.consumeClickAfterLongPress()) {
            event.preventDefault();
            return;
          }
          if (selectionMode) {
            event.preventDefault();
            onSelectionToggle?.(file);
            return;
          }
          if (viewBusy) {
            event.preventDefault();
            return;
          }
          if (!file.is_dir) return;
          if (onFolderEnter?.(file) === false) {
            event.preventDefault();
          }
        }}
      >
        <span className={styles.iconBox}>
          {entering ? (
            <LoadingSpinnerIcon className={styles.loadingIcon} />
          ) : (
            <MobileFileIcon
              file={file}
              iconName={presentation.iconName}
              hasThumbnail={
                file.status === "ready" &&
                Boolean(file.metadata?.thumbnail_path) &&
                (presentation.kind === "image" || presentation.kind === "video")
              }
            />
          )}
        </span>
        <span className={styles.text}>
          <strong>{file.name}</strong>
          <span>
            {entering
              ? t("fileList.enteringFolder")
              : file.is_dir
                ? presentation.description
                : fileSizeLabel(file)}
          </span>
        </span>
      </Link>
      {selectionMode ? (
        <span className={styles.selectionMark} aria-hidden>
          <span className="material-symbols-outlined">
            {selected ? "check_circle" : "radio_button_unchecked"}
          </span>
        </span>
      ) : (
        <button
          className={styles.moreButton}
          type="button"
          aria-label={t("mobile.files.moreActions", { name: file.name })}
          disabled={viewBusy}
          onClick={() => {
            if (!viewBusy) onMore?.(file);
          }}
        >
          <span className="material-symbols-outlined" aria-hidden>
            more_vert
          </span>
        </button>
      )}
    </article>
  );
}

function LoadingSpinnerIcon({ className = "" }: { className?: string }) {
  return (
    <svg
      className={className}
      viewBox="0 0 24 24"
      fill="none"
      aria-hidden="true"
      focusable="false"
    >
      <circle
        cx="12"
        cy="12"
        r="9"
        stroke="currentColor"
        strokeWidth="3"
        opacity="0.18"
      />
      <path
        d="M21 12a9 9 0 0 0-9-9"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="3"
      />
    </svg>
  );
}

function MobileFileIcon({
  file,
  iconName,
  hasThumbnail,
}: {
  file: DriveFile;
  iconName: string;
  hasThumbnail: boolean;
}) {
  const icon = (
    <span className="material-symbols-outlined" aria-hidden>
      {iconName}
    </span>
  );
  if (hasThumbnail) {
    return (
      <LazyThumbnail
        file={file}
        className={styles.thumbnail}
        fallback={icon}
      />
    );
  }

  return icon;
}
