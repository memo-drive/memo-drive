import type { DriveFile } from "../../types";
import { Button } from "../base";
import styles from "./TrashList.module.css";

interface Props {
  files: DriveFile[];
  loading: boolean;
  actionId?: string;
  onRestore: (file: DriveFile) => void;
  onPurge: (file: DriveFile) => void;
}

export function TrashList({
  files,
  loading,
  actionId,
  onRestore,
  onPurge,
}: Props) {
  if (loading && files.length === 0) {
    return (
      <div className={styles.empty}>
        <span className="material-symbols-outlined">hourglass_empty</span>
        <p>正在加载回收站…</p>
      </div>
    );
  }

  if (files.length === 0) {
    return (
      <div className={styles.empty}>
        <span className="material-symbols-outlined">delete</span>
        <h3>回收站是空的</h3>
        <p>移到回收站的文件会先在这里保留一段时间。</p>
      </div>
    );
  }

  return (
    <div className={styles.tableWrapper}>
      <table className={styles.table}>
        <thead>
          <tr className={styles.tableHeadRow}>
            <th className={styles.tableHeadCell}>名称</th>
            <th className={styles.tableHeadCell}>原路径</th>
            <th className={styles.tableHeadCell}>删除时间</th>
            <th className={styles.tableHeadCellRight}>操作</th>
          </tr>
        </thead>
        <tbody>
          {files.map((file) => (
            <tr key={file.id} className={styles.tableRow}>
              <td className={styles.tableCell}>
                <div className={styles.fileIconWrapper}>
                  <div className={`${styles.iconBox} ${iconClass(file)}`}>
                    <span className="material-symbols-outlined">
                      {iconName(file)}
                    </span>
                  </div>
                  <div>
                    <p className={styles.fileName}>{displayName(file)}</p>
                    <p className={styles.fileDesc}>
                      {file.is_dir ? "Folder" : file.mime_type || formatSize(file.size)}
                    </p>
                  </div>
                </div>
              </td>
              <td className={styles.tableCell}>
                <span className={styles.metaText}>{file.original_path ?? file.path}</span>
              </td>
              <td className={styles.tableCell}>
                <span className={styles.metaText}>{formatDate(file.deleted_at)}</span>
              </td>
              <td className={styles.tableCellRight}>
                <div className={styles.actions}>
                  <Button
                    size="sm"
                    variant="ghost"
                    loading={actionId === `restore:${file.id}`}
                    onClick={() => onRestore(file)}
                  >
                    恢复
                  </Button>
                  <Button
                    size="sm"
                    variant="danger"
                    loading={actionId === `purge:${file.id}`}
                    onClick={() => onPurge(file)}
                  >
                    永久删除
                  </Button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export function displayName(file: DriveFile) {
  return file.original_name ?? file.name.replace(`${file.id}-`, "");
}

function iconClass(file: DriveFile) {
  if (file.is_dir) return styles.iconBoxDir;
  if (file.mime_type?.startsWith("image/")) return styles.iconBoxImg;
  return styles.iconBoxFile;
}

function iconName(file: DriveFile) {
  if (file.is_dir) return "folder";
  if (file.mime_type?.startsWith("image/")) return "image";
  if (file.mime_type?.startsWith("video/")) return "video_library";
  if (file.mime_type?.startsWith("audio/")) return "audio_file";
  return "description";
}

function formatDate(value?: string) {
  if (!value) return "--";
  return new Date(value).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1024 * 1024 * 1024) {
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
  return `${(size / 1024 / 1024 / 1024).toFixed(2)} GB`;
}
