import { useTranslation } from "react-i18next";
import type { DriveFile } from "../../types";
import styles from "./MobileMoveTargetPrompt.module.css";

interface MobileMoveTargetPromptProps {
  open: boolean;
  title: string;
	mode?: "move" | "copy";
  currentDir: string;
  dirs: DriveFile[];
  loading?: boolean;
  loadingMore?: boolean;
  hasMore?: boolean;
  busy?: boolean;
  disabledReason?: string;
  onClose: () => void;
  onMoveHere: () => void;
  onEnterDir: (dir: DriveFile) => void;
  onGoToDir: (path: string) => void;
  onLoadMore?: () => void;
}

export function MobileMoveTargetPrompt({
  open,
  title,
	mode = "move",
  currentDir,
  dirs,
  loading = false,
  loadingMore = false,
  hasMore = false,
  busy = false,
  disabledReason = "",
  onClose,
  onMoveHere,
  onEnterDir,
  onGoToDir,
  onLoadMore,
}: MobileMoveTargetPromptProps) {
  const { t } = useTranslation();
  if (!open) return null;
  const disabled = Boolean(disabledReason) || busy;

  return (
    <section className={styles.sheet} data-mobile-move-target="light" role="dialog" aria-modal="true">
      <button className={styles.backdrop} type="button" aria-label={t("common.close")} onClick={onClose} />
      <div className={styles.panel}>
        <div className={styles.handle} />
        <header className={styles.header}>
          <strong>{title}</strong>
          <button type="button" onClick={onClose} aria-label={t("common.close")}>
            <span className="material-symbols-outlined" aria-hidden>
              close
            </span>
          </button>
        </header>
        <div className={styles.breadcrumbs}>
          {breadcrumbs(currentDir, t("drive.rootDir")).map((crumb, index, list) => (
            <span key={crumb.path}>
              <button type="button" onClick={() => onGoToDir(crumb.path)}>
                {crumb.label}
              </button>
              {index < list.length - 1 ? <em>/</em> : null}
            </span>
          ))}
        </div>
        <div className={styles.currentBox}>
          <span className="material-symbols-outlined" aria-hidden>
            folder_open
          </span>
          <span>
            <small>{t("moveDialog.currentLocation")}</small>
            <strong>{currentDir}</strong>
          </span>
        </div>
        {disabledReason ? <p className={styles.warning}>{disabledReason}</p> : null}
        <div className={styles.dirList}>
          {loading ? <div className={styles.empty}>{t("moveDialog.loadingDirs")}</div> : null}
          {!loading && dirs.length === 0 ? <div className={styles.empty}>{t("moveDialog.emptyDirs")}</div> : null}
          {!loading
            ? dirs.map((dir) => (
                <button key={dir.id} className={styles.dirRow} type="button" onClick={() => onEnterDir(dir)}>
                  <span className="material-symbols-outlined" aria-hidden>
                    folder
                  </span>
                  <span>{dir.name}</span>
                  <span className="material-symbols-outlined" aria-hidden>
                    chevron_right
                  </span>
                </button>
              ))
            : null}
          {!loading && hasMore ? (
            <button className={styles.dirRow} type="button" disabled={loadingMore} onClick={onLoadMore}>
              <span className="material-symbols-outlined" aria-hidden>
                expand_more
              </span>
              <span>{loadingMore ? t("moveDialog.loadingDirs") : t("moveDialog.loadMore")}</span>
            </button>
          ) : null}
        </div>
        <footer className={styles.footer}>
          <button type="button" onClick={onClose}>
            {t("common.cancel")}
          </button>
          <button className={styles.primary} type="button" disabled={disabled} onClick={onMoveHere}>
			{busy ? t("drive.processing") : t(mode === "copy" ? "copyDialog.copyHere" : "moveDialog.moveHere")}
          </button>
        </footer>
      </div>
    </section>
  );
}

function breadcrumbs(path: string, rootLabel: string) {
  const parts = path.split("/").filter(Boolean);
  return [{ label: rootLabel, path: "/" }].concat(
    parts.map((part, index) => ({
      label: part,
      path: `/${parts.slice(0, index + 1).join("/")}`,
    })),
  );
}
