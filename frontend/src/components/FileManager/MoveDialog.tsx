import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { copyFile, listFiles, moveFile } from "../../api/fileApi";
import type { DriveFile } from "../../types";
import { buildCopyRequest } from "../../workflows/copyWorkflow";
import { appendFolderPage } from "../../workflows/folderPagination";
import { Button, message, Modal } from "../base";
import styles from "./MoveDialog.module.css";

interface Props {
  open: boolean;
  target: DriveFile | null;
	mode?: "move" | "copy";
  onClose: () => void;
  onMoved: () => void | Promise<void>;
}

export function MoveDialog({ open, target, mode = "move", onClose, onMoved }: Props) {
  const { t } = useTranslation();
  const [currentDir, setCurrentDir] = useState("/");
  const [dirs, setDirs] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState("");
  const [hasMore, setHasMore] = useState(false);
  const [moving, setMoving] = useState(false);
  const currentDirRef = useRef(currentDir);
  currentDirRef.current = currentDir;

  useEffect(() => {
    if (!open) return;
    setCurrentDir("/");
  }, [open, target?.id]);

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    listFiles(currentDir, { sort: "name", limit: 100 })
      .then((response) => {
        if (cancelled) return;
		setDirs(response.files.filter((file) => file.is_dir && (mode === "copy" || file.id !== target?.id)));
        setNextCursor(response.next_cursor);
        setHasMore(response.has_more && response.files.every((file) => file.is_dir));
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
	}, [open, currentDir, mode, target?.id]);

  async function loadMoreDirs() {
    if (!nextCursor || !hasMore || loadingMore) return;
    const path = currentDir;
    setLoadingMore(true);
    try {
      const response = await listFiles(path, { sort: "name", cursor: nextCursor, limit: 100 });
      if (currentDirRef.current !== path) return;
		const nextDirs = response.files.filter((file) => file.is_dir && (mode === "copy" || file.id !== target?.id));
      setDirs((current) => appendFolderPage(current, nextDirs));
      setNextCursor(response.next_cursor);
      setHasMore(response.has_more && response.files.every((file) => file.is_dir));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("drive.loadError"));
    } finally {
      setLoadingMore(false);
    }
  }

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

	const reason = mode === "move" ? disabledReason(target, currentDir) : "";

	async function submitOperation() {
    if (!target || reason) return;
    setMoving(true);
    try {
		if (mode === "copy") {
			const request = buildCopyRequest(target, currentDir);
			await copyFile(request.id, request.input);
		} else {
			await moveFile(target.id, currentDir);
		}
		message.success(t(mode === "copy" ? "copyDialog.success" : "moveDialog.success"));
      onClose();
      await onMoved();
    } catch (err) {
		message.error(isHTTPStatus(err, 409) ? t("moveDialog.targetExists") : t(mode === "copy" ? "copyDialog.failed" : "moveDialog.failed"));
    } finally {
      setMoving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
		title={t(mode === "copy" ? "copyDialog.title" : "moveDialog.title", { name: target.name })}
      width={560}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {t("common.cancel")}
          </Button>
		  <Button variant="primary" onClick={submitOperation} disabled={!!reason} loading={moving}>
			{t(mode === "copy" ? "copyDialog.copyHere" : "moveDialog.moveHere")}
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
          {!loading && hasMore ? (
            <Button variant="ghost" loading={loadingMore} onClick={loadMoreDirs}>
              {t("moveDialog.loadMore")}
            </Button>
          ) : null}
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
