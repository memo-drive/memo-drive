import { useEffect, useState } from "react";
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
        message.error(err instanceof Error ? err.message : "加载目录失败");
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

  const reason = disabledReason(target, currentDir);

  async function submitMove() {
    if (!target || reason) return;
    setMoving(true);
    try {
      await moveFile(target.id, currentDir);
      message.success("已移动");
      onClose();
      await onMoved();
    } catch (err) {
      message.error(isHTTPStatus(err, 409) ? "目标位置已存在同名文件/目录" : "移动失败");
    } finally {
      setMoving(false);
    }
  }

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={`移动 ${target.name}`}
      width={560}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            取消
          </Button>
          <Button variant="primary" onClick={submitMove} disabled={!!reason} loading={moving}>
            移到这里
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
            <strong>当前位置</strong>
            <p>{currentDir}</p>
          </div>
        </div>

        {reason ? <p className={styles.warning}>{reason}</p> : null}

        <div className={styles.dirList}>
          {loading ? <div className={styles.empty}>正在加载目录...</div> : null}
          {!loading && dirs.length === 0 ? <div className={styles.empty}>这个目录下没有子文件夹</div> : null}
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

function breadcrumbs(path: string) {
  const parts = path.split("/").filter(Boolean);
  return [{ label: "根目录", path: "/" }].concat(
    parts.map((part, index) => ({
      label: part,
      path: "/" + parts.slice(0, index + 1).join("/"),
    })),
  );
}

function disabledReason(target: DriveFile, currentDir: string) {
  if (currentDir === target.path) {
    return "文件已经在这个目录中";
  }
  if (!target.is_dir) return "";
  const targetVirtual = joinVirtualPath(target.path, target.name);
  if (currentDir === targetVirtual || currentDir.startsWith(`${targetVirtual}/`)) {
    return "不能把目录移动到它自己或子目录中";
  }
  return "";
}

function joinVirtualPath(base: string, name: string) {
  const cleanBase = base === "/" ? "" : base.replace(/\/+$/, "");
  return `${cleanBase}/${name}` || "/";
}

function isHTTPStatus(err: unknown, status: number) {
  return typeof err === "object" && err !== null && "status" in err && (err as { status?: number }).status === status;
}
