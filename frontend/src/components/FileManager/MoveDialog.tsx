import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { listFiles, moveFile } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { Button, message, Modal } from "../base";
import styles from "./MoveDialog.module.css";

interface Props {
  open: boolean;
  target: DriveFile | null;
  onClose: () => void;
  onMoved: () => void | Promise<void>;
}

export function MoveDialog({ open, target, onClose, onMoved }: Props) {
  const { t } = useTranslation();
  const [currentDir, setCurrentDir] = useState("/");
  const [dirs, setDirs] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [moving, setMoving] = useState(false);

  useEffect(() => {
    if (!open) return;
    setCurrentDir("/");
  }, [open, target?.id]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    listFiles(currentDir)
      .then((response) => {
        if (cancelled) return;
        setDirs(response.files.filter((file) => file.is_dir && file.id !== target?.id));
      })
      .catch((err) => {
        if (cancelled) return;
        message.error(err instanceof Error ? err.message : t("drive.loadError"));
        setDirs([]);
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open, currentDir, target?.id]);

  if (!target) return null;

  function breadcrumbs(path: string) {
    const parts = path.split("/").filter(Boolean);
    return [{ label: t("drive.rootDir"), path: "/" }].concat(
      parts.map((part, index) => ({
        label: part,
        path: "/" + parts.slice(0, index + 1).join("/"),
      })),
    );
  }

  function disabledReason(target: DriveFile, currentDir: string) {
    if (currentDir === target.path) {
      return t("moveDialog.alreadyHere");
    }
    if (!target.is_dir) return "";
    const targetVirtual = joinVirtualPath(target.path, target.name);
    if (currentDir === targetVirtual || currentDir.startsWith(`${targetVirtual}/`)) {
      return t("moveDialog.cannotMoveToSelf");
    }
    return "";
  }

  const reason = disabledReason(target, currentDir);

  async function submitMove() {
    if (!target || reason) return;
    setMoving(true);
    try {
      await moveFile(target.id, currentDir);
      message.success(t("moveDialog.success"));
      onClose();
      await onMoved();
    } catch (err) {
      message.error(isHTTPStatus(err, 409) ? t("moveDialog.targetExists") : t("moveDialog.failed"));
    } finally {
      setMoving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t("moveDialog.title", { name: target.name })}
      width={560}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button variant="primary" onClick={submitMove} disabled={!!reason} loading={moving}>
            {t("moveDialog.moveHere")}
          </Button>
        </>
      }
    >
      <div className={styles.dialog}>
        <div className={styles.breadcrumbs}>
          {breadcrumbs(currentDir).map((crumb, index, list) => (
            <span key={crumb.path} className={styles.crumb}>
              <button
                type="button"
                className={styles.crumbBtn}
                onClick={() => setCurrentDir(crumb.path)}
              >
                {crumb.label}
              </button>
              {index < list.length - 1 ? <span className={styles.sep}>/</span> : null}
            </span>
          ))}
        </div>

        <div className={styles.currentBox}>
          <span className="material-symbols-outlined">folder_open</span>
          <div>
            <strong>{t("moveDialog.currentLocation")}</strong>
            <p>{currentDir}</p>
          </div>
        </div>

        {reason ? <p className={styles.warning}>{reason}</p> : null}

        <div className={styles.dirList}>
          {loading ? <div className={styles.empty}>{t("moveDialog.loadingDirs")}</div> : null}
          {!loading && dirs.length === 0 ? <div className={styles.empty}>{t("moveDialog.emptyDirs")}</div> : null}
          {!loading
            ? dirs.map((dir) => (
                <button
                  key={dir.id}
                  type="button"
                  className={styles.dirRow}
                  onClick={() => setCurrentDir(joinVirtualPath(dir.path, dir.name))}
                >
                  <span className="material-symbols-outlined">folder</span>
                  <span>{dir.name}</span>
                  <span className="material-symbols-outlined">chevron_right</span>
                </button>
              ))
            : null}
        </div>
      </div>
    </Modal>
  );
}

function joinVirtualPath(base: string, name: string) {
  const cleanBase = base === "/" ? "" : base.replace(/\/+$/, "");
  return `${cleanBase}/${name}` || "/";
}

function isHTTPStatus(err: unknown, status: number) {
  return typeof err === "object" && err !== null && "status" in err && (err as { status?: number }).status === status;
}
