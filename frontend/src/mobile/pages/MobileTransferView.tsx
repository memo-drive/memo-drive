import { useTranslation } from "react-i18next";
import {
  directoryTransferSummaries,
  isActiveTransferStatus,
  transferStatusLabelKey,
} from "../../stores/transferProjection";
import type { TransferTask } from "../../stores/transferStore";
import { formatBytes } from "../../utils/formatBytes";
import { formatTransferSpeed, transferUploadedBytes } from "../../utils/uploadProgress";
import styles from "./MobileTransferView.module.css";

interface MobileTransferViewProps {
  tasks: TransferTask[];
  loading?: boolean;
  onPause?: (id: string) => void;
  onResume?: (task: TransferTask) => void;
  onRetryConflict?: (
    task: TransferTask,
    policy: "rename" | "replace",
  ) => void;
  onCancel?: (id: string) => void;
  onRemove?: (id: string) => void;
  onClearDone?: () => void;
}

export function MobileTransferView({
  tasks,
  loading = false,
  onPause,
  onResume,
  onRetryConflict,
  onCancel,
  onRemove,
  onClearDone,
}: MobileTransferViewProps) {
  const { t } = useTranslation();
  const hasDone = tasks.some((task) => !isActiveTransferStatus(task.status));
  const directorySummaries = directoryTransferSummaries(tasks);

  if (loading && tasks.length === 0) {
    return <div className={styles.state}>{t("transfer.syncing")}</div>;
  }

  if (tasks.length === 0) {
    return <div className={styles.state}>{t("transfer.emptyActive")}</div>;
  }

  return (
    <div className={styles.list}>
      {directorySummaries.map((group) => (
        <section key={group.batchId} className={styles.directorySummary}>
          <div>
            <strong>{group.name}</strong>
            <span>
              {t("transfer.directoryFiles", {
                completed: group.completedCount,
                total: group.fileCount,
              })}
            </span>
          </div>
          <div className={styles.progressTrack}>
            <span style={{ width: `${group.percent}%` }} />
          </div>
          <b>{group.percent}%</b>
        </section>
      ))}
      {hasDone && onClearDone ? (
        <button className={styles.clearButton} type="button" onClick={onClearDone}>
          <span className="material-symbols-outlined" aria-hidden>
            sweep
          </span>
          {t("transfer.clearAll")}
        </button>
      ) : null}
      {tasks.map((task) => {
        const isDone = task.status === "done";
        const requestedName = task.requestedName ?? task.fileName;
        const resolvedName = task.resolvedName ?? task.fileName;
        const fileSizeText = formatBytes(task.fileSize);
        const uploadedBytes = transferUploadedBytes(task);
        const speed = formatTransferSpeed(task.speed);
        const progressText = `${formatBytes(uploadedBytes)} / ${fileSizeText}`;
        const waitingForProgress =
          task.status === "uploading" && task.percent === 0 && !speed;
        return (
          <article key={task.id} className={styles.card}>
            <div className={styles.cardHeader}>
              <span className="material-symbols-outlined" aria-hidden>
                upload
              </span>
              <div>
                <h2>{resolvedName}</h2>
                <p>{task.destPath}</p>
              </div>
              <strong>{t(transferStatusLabelKey(task.status))}</strong>
            </div>
            {!isDone ? (
              <div className={styles.progressTrack}>
                <span style={{ width: `${Math.max(0, Math.min(100, task.percent))}%` }} />
              </div>
            ) : null}
            <p className={styles.meta}>
              {isDone ? (
                <span>{fileSizeText}</span>
              ) : (
                <>
                  <span>{task.percent}%</span>
                  <span>{progressText}</span>
                  {task.status === "uploading" && speed ? <span>{speed}</span> : null}
                  {waitingForProgress ? <span>{t("transfer.preparingUpload")}</span> : null}
                </>
              )}
            </p>
            {requestedName !== resolvedName ? (
              <p className={styles.resolvedName}>
                {t("transfer.resolvedName", {
                  requested: requestedName,
                  resolved: resolvedName,
                })}
              </p>
            ) : null}
            {task.error ? <p className={styles.error}>{task.error}</p> : null}
            <div className={styles.actions}>
              {task.status === "uploading" ? (
                <>
                  <button type="button" onClick={() => onPause?.(task.id)}>
                    {t("transfer.actionPause")}
                  </button>
                  <button type="button" onClick={() => onCancel?.(task.id)}>
                    {t("transfer.actionCancel")}
                  </button>
                </>
              ) : null}
              {task.status === "paused" ? (
                <>
                  <button type="button" onClick={() => onResume?.(task)}>
                    {t("transfer.actionResume")}
                  </button>
                  <button type="button" onClick={() => onCancel?.(task.id)}>
                    {t("transfer.actionCancel")}
                  </button>
                </>
              ) : null}
              {task.status === "failed" && task.conflictRetryable ? (
                <>
                  <button
                    type="button"
                    onClick={() => onRetryConflict?.(task, "rename")}
                  >
                    {t("transfer.retryKeepBoth")}
                  </button>
                  <button
                    type="button"
                    onClick={() => onRetryConflict?.(task, "replace")}
                  >
                    {t("transfer.retryReplace")}
                  </button>
                </>
              ) : null}
              {!isActiveTransferStatus(task.status) ? (
                <button type="button" onClick={() => onRemove?.(task.id)}>
                  {t("transfer.actionRemove")}
                </button>
              ) : null}
            </div>
          </article>
        );
      })}
    </div>
  );
}
