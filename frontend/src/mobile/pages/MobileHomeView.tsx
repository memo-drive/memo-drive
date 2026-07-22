import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../types";
import { filePresentation } from "../../components/FileManager/filePresentation";
import { LazyThumbnail } from "../../components/Virtualized";
import { UploadFab } from "../components/UploadFab/UploadFab";
import { mobilePreviewHref, normalizeMobilePath } from "../utils/mobilePath";
import styles from "./MobileHomeView.module.css";

interface MobileHomeViewProps {
  searchDraft: string;
  transferCount?: number;
  recentFiles: DriveFile[];
  searchActive?: boolean;
  searchResults?: DriveFile[];
  searching?: boolean;
  recentLoading?: boolean;
  searchError?: string;
  onSearchDraftChange?: (value: string) => void;
  onSearchSubmit?: () => void;
  onClearSearch?: () => void;
  onUploadFiles?: (files: FileList | null) => void;
}

const categoryShortcuts = [
  { key: "photos", href: "/m/category/photos", icon: "image", labelKey: "mobile.home.categories.photos" },
  { key: "videos", href: "/m/category/videos", icon: "videocam", labelKey: "mobile.home.categories.videos" },
  { key: "documents", href: "/m/category/documents", icon: "description", labelKey: "mobile.home.categories.documents" },
  { key: "audio", href: "/m/category/audio", icon: "music_note", labelKey: "mobile.home.categories.audio" },
];

export function MobileHomeView({
  searchDraft,
  transferCount = 0,
  recentFiles,
  searchActive = false,
  searchResults = [],
  searching = false,
  recentLoading = false,
  searchError = "",
  onSearchDraftChange,
  onSearchSubmit,
  onClearSearch,
  onUploadFiles,
}: MobileHomeViewProps) {
  const { t } = useTranslation();
  const hasSearchDraft = searchDraft.trim().length > 0;

  return (
    <section className={styles.page} data-mobile-page="home">
      <header className={styles.header}>
        <h1>{t("mobile.home.title")}</h1>
        <Link className={styles.transferButton} to="/m/transfer" aria-label={t("mobile.home.transfer")}>
          <span className="material-symbols-outlined" aria-hidden>
            swap_vert
          </span>
          {transferCount > 0 ? <strong>{transferCount}</strong> : null}
        </Link>
      </header>

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
          placeholder={t("mobile.home.searchPlaceholder")}
          onChange={(event) => onSearchDraftChange?.(event.target.value)}
        />
        {searchActive || hasSearchDraft ? (
          <button type="button" onClick={onClearSearch} aria-label={t("searchResultList.clearSearch")}>
            <span className="material-symbols-outlined" aria-hidden>
              close
            </span>
          </button>
        ) : null}
      </form>

      <nav className={styles.categories} aria-label={t("mobile.home.categoriesLabel")}>
        {categoryShortcuts.map((item) => (
          <Link key={item.key} to={item.href}>
            <span className="material-symbols-outlined" aria-hidden>
              {item.icon}
            </span>
            <strong>{t(item.labelKey)}</strong>
          </Link>
        ))}
      </nav>

      {searchActive ? (
        <FileRail
          title={t("mobile.home.searchResults")}
          files={searchResults}
          loading={searching}
          emptyText={t("searchResultList.noResults")}
          error={searchError}
        />
      ) : (
        <FileRail
          title={t("mobile.home.recent")}
          files={recentFiles}
          loading={recentLoading}
          emptyText={t("mobile.home.recentEmpty")}
        />
      )}

      <UploadFab onFiles={onUploadFiles} />
    </section>
  );
}

function FileRail({
  title,
  files,
  loading,
  emptyText,
  error = "",
}: {
  title: string;
  files: DriveFile[];
  loading: boolean;
  emptyText: string;
  error?: string;
}) {
  const { t } = useTranslation();

  return (
    <section className={styles.fileSection}>
      <header>
        <h2>{title}</h2>
      </header>
      {loading && files.length === 0 ? (
        <div className={styles.state}>{t("common.loading")}</div>
      ) : error ? (
        <div className={styles.state}>{error}</div>
      ) : files.length === 0 ? (
        <div className={styles.state}>{emptyText}</div>
      ) : (
        <div className={styles.fileGrid}>
          {files.map((file) => (
            <Link key={file.id} className={styles.fileItem} to={mobileHomeFileHref(file)}>
              <HomeFileVisual file={file} />
              <span>
                <strong>{file.name}</strong>
                <small>{normalizeMobilePath(file.path || "/")}</small>
              </span>
            </Link>
          ))}
        </div>
      )}
    </section>
  );
}

function HomeFileVisual({ file }: { file: DriveFile }) {
  const presentation = filePresentation(file);
  const icon = (
    <span className="material-symbols-outlined" aria-hidden>
      {homeFileIcon(file)}
    </span>
  );
  const showThumbnail = presentation.kind === "image" || presentation.kind === "video";

  return (
    <span className={styles.fileIcon}>
      {showThumbnail ? (
        <LazyThumbnail
          file={file}
          className={styles.fileThumb}
          placeholderClassName={styles.fileThumbPlaceholder}
          fallback={icon}
        />
      ) : (
        icon
      )}
    </span>
  );
}

function mobileHomeFileHref(file: DriveFile) {
  const path = normalizeMobilePath(file.path || "/");
  const category = mediaCategory(file);
  if (category) {
    return `/m/media/${category}/${encodeURIComponent(file.id)}?returnTo=${encodeURIComponent("/m")}`;
  }
  return mobilePreviewHref(file.id, path);
}

function mediaCategory(file: DriveFile): "photos" | "videos" | "audio" | null {
  const mime = file.mime_type.toLowerCase();
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  if (mime.startsWith("image/") || ["jpg", "jpeg", "png", "gif", "webp", "heic", "heif", "avif"].includes(ext)) {
    return "photos";
  }
  if (mime.startsWith("video/") || ["mp4", "mov", "m4v", "mkv", "webm", "flv", "avi"].includes(ext)) {
    return "videos";
  }
  if (mime.startsWith("audio/") || ["mp3", "m4a", "aac", "wav", "flac", "ogg", "opus"].includes(ext)) {
    return "audio";
  }
  return null;
}

function homeFileIcon(file: DriveFile) {
  const category = mediaCategory(file);
  if (category === "photos") return "image";
  if (category === "videos") return "movie";
  if (category === "audio") return "headphones";
  return file.is_dir ? "folder" : "description";
}
