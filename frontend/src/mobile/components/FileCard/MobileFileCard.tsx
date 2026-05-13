import { useState } from "react";
import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { thumbnailUrl } from "../../../api/fileApi";
import type { DriveFile } from "../../../types";
import { filePresentation, fileSizeLabel } from "../../../components/FileManager/filePresentation";
import { mobileFilesHref, mobilePreviewHref, normalizeMobilePath } from "../../utils/mobilePath";
import styles from "./MobileFileCard.module.css";

interface MobileFileCardProps {
  file: DriveFile;
  currentPath: string;
  onMore?: (file: DriveFile) => void;
}

export function MobileFileCard({ file, currentPath, onMore }: MobileFileCardProps) {
  const { t } = useTranslation();
  const presentation = filePresentation(file);
  const href = file.is_dir
    ? mobileFilesHref(joinMobileFolderPath(currentPath, file.name))
    : mobilePreviewHref(file.id, currentPath);

  return (
    <article className={styles.card}>
      <Link className={styles.main} to={href}>
        <span className={styles.iconBox}>
          <MobileFileIcon
            file={file}
            iconName={presentation.iconName}
            hasThumbnail={
              file.status === "ready" &&
              Boolean(file.metadata?.thumbnail_path) &&
              (presentation.kind === "image" || presentation.kind === "video")
            }
          />
        </span>
        <span className={styles.text}>
          <strong>{file.name}</strong>
          <span>{file.is_dir ? presentation.description : fileSizeLabel(file)}</span>
        </span>
      </Link>
      <button
        className={styles.moreButton}
        type="button"
        aria-label={t("mobile.files.moreActions", { name: file.name })}
        onClick={() => onMore?.(file)}
      >
        <span className="material-symbols-outlined" aria-hidden>
          more_vert
        </span>
      </button>
    </article>
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
  const [thumbnailFailed, setThumbnailFailed] = useState(false);
  if (hasThumbnail && !thumbnailFailed) {
    return (
      <img
        className={styles.thumbnail}
        src={thumbnailUrl(file.id)}
        alt={file.name}
        loading="lazy"
        decoding="async"
        onError={() => setThumbnailFailed(true)}
      />
    );
  }

  return (
    <span className="material-symbols-outlined" aria-hidden>
      {iconName}
    </span>
  );
}

function joinMobileFolderPath(base: string, name: string) {
  const normalized = normalizeMobilePath(base);
  return normalizeMobilePath(`${normalized === "/" ? "" : normalized}/${name}`);
}
