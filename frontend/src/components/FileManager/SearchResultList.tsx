import { useTranslation } from "react-i18next";
import type { DriveFile, FileMatchType, FileSearchHit } from "../../types";
import {
  filePresentation,
  fileSizeLabel,
  type FilePresentationKind,
} from "./filePresentation";
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
  const iconClasses: Record<FilePresentationKind, string> = {
    audio: styles.iconAudio,
    file: styles.iconFile,
    folder: styles.iconDir,
    image: styles.iconImage,
    video: styles.iconVideo,
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
        {hits.map((hit) => {
          const presentation = filePresentation(hit.file);
          return (
            <button
              key={hit.file.id}
              type="button"
              className={styles.resultRow}
              onClick={() => onPick(hit.file)}
            >
              <div className={`${styles.iconBox} ${iconClasses[presentation.kind]}`}>
                <span className="material-symbols-outlined">{presentation.iconName}</span>
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
                  <span>{hit.file.is_dir ? presentation.description : fileSizeLabel(hit.file)}</span>
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
          );
        })}
      </div>
    </div>
  );
}
