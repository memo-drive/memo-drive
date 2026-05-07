import { useTranslation } from "react-i18next";
import type { DriveFile, FileMatchType, FileSearchHit } from "../../types";
import styles from "./SearchResultList.module.css";

interface Props {
  hits: FileSearchHit[];
  loading?: boolean;
  onClear: () => void;
  onPick: (file: DriveFile) => void;
}

export function SearchResultList({ hits, loading = false, onClear, onPick }: Props) {
  const { t } = useTranslation();

  const matchLabels: Record<FileMatchType, string> = {
    name: t("searchResultList.matchName"),
    meta: t("searchResultList.matchMeta"),
    semantic: t("searchResultList.matchContent"),
    filter: t("searchResultList.matchLabel"),
  };

  return (
    <div className={styles.wrapper}>
      <div className={styles.infoBar}>
        <div>
          <span className={styles.infoTitle}>
            {loading ? t("searchResultList.searching") : t("searchResultList.foundResults", { count: hits.length })}
          </span>
          <span className={styles.infoHint}>{t("searchResultList.sortHint")}</span>
        </div>
        <button className={styles.clearBtn} onClick={onClear}>
          {t("searchResultList.clearSearch")}
        </button>
      </div>

      {!loading && hits.length === 0 ? (
        <div className={styles.emptyState}>{t("searchResultList.noResults")}</div>
      ) : null}

      <div className={styles.list}>
        {hits.map((hit) => (
          <button
            key={hit.file.id}
            type="button"
            className={styles.resultRow}
            onClick={() => onPick(hit.file)}
          >
            <div className={`${styles.iconBox} ${iconClass(hit.file)}`}>
              <span className="material-symbols-outlined">{iconName(hit.file)}</span>
            </div>
            <div className={styles.resultMain}>
              <div className={styles.titleLine}>
                <span className={styles.fileName}>{hit.file.name}</span>
                <span className={styles.score}>{Math.round(hit.score * 100)}%</span>
              </div>
              <div className={styles.metaLine}>
                <span>{hit.file.path}</span>
                <span>·</span>
                <span>{new Date(hit.file.updated_at).toLocaleString()}</span>
                <span>·</span>
                <span>{hit.file.is_dir ? "Folder" : formatSize(hit.file.size)}</span>
              </div>
              {hit.snippet ? <p className={styles.snippet}>{hit.snippet}</p> : null}
              <div className={styles.badges}>
                {hit.match_types.map((type) => (
                  <span key={type} className={`${styles.badge} ${styles[`badge_${type}`]}`}>
                    {matchLabels[type]}
                  </span>
                ))}
              </div>
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

function iconClass(file: DriveFile) {
  if (file.is_dir) return styles.iconDir;
  if (file.mime_type.startsWith("image/")) return styles.iconImage;
  if (file.mime_type.startsWith("video/")) return styles.iconVideo;
  if (file.mime_type.startsWith("audio/")) return styles.iconAudio;
  return styles.iconFile;
}

function iconName(file: DriveFile) {
  if (file.is_dir) return "folder";
  if (file.mime_type.startsWith("image/")) return "image";
  if (file.mime_type.startsWith("video/")) return "video_library";
  if (file.mime_type.startsWith("audio/")) return "audio_file";
  return "description";
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / 1024 / 1024).toFixed(1)} MB`;
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
