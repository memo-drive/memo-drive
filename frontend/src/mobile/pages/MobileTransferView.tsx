import { useTranslation } from "react-i18next";
import type { TransferStatus, TransferTask } from "../../stores/transferStore";
import styles from "./MobileTransferView.module.css";

interface MobileTransferViewProps {
  tasks: TransferTask[];
  loading?: boolean;
  onPause?: (id: string) => void;
  onResume?: (task: TransferTask) => void;
  onCancel?: (id: string) => void;
  onRemove?: (id: string) => void;
  onClearDone?: () => void;
}

export function MobileTransferView({
  tasks,
  loading = false,
  onPause,
  onResume,
  onCancel,
  onRemove,
  onClearDone,
}: MobileTransferViewProps) {
  const { t } = useTranslation();
  const hasDone = tasks.some((task) => !isActiveStatus(task.status));

  if (loading && tasks.length === 0) {
    return <div className={styles.state}>{t("transfer.syncing")}</div>;
  }

  if (tasks.length === 0) {
    return <div className={styles.state}>{t("transfer.emptyActive")}</div>;
  }

  return (
    <div className={styles.list}>
      {hasDone && onClearDone ? (
        <button className={styles.clearButton} type="button" onClick={onClearDone}>
          <span className="material-symbols-outlined" aria-hidden>
            sweep
          </span>
          {t("transfer.clearAll")}
        </button>
      ) : null}
      {tasks.map((task) => (
        <article key={task.id} className={styles.card}>
          <div className={styles.cardHeader}>
            <span className="material-symbols-outlined" aria-hidden>
              upload
            </span>
            <div>
              <h2>{task.fileName}</h2>
              <p>{task.destPath}</p>
            </div>
            <strong>{statusLabel(task.status, t)}</strong>
          </div>
          <div className={styles.progressTrack}>
            <span style={{ width: `${Math.max(0, Math.min(100, task.percent))}%` }} />
          </div>
          <p className={styles.meta}>{task.percent}%</p>
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
            {!isActiveStatus(task.status) ? (
              <button type="button" onClick={() => onRemove?.(task.id)}>
                {t("transfer.actionRemove")}
              </button>
            ) : null}
          </div>
        </article>
      ))}
    </div>
  );
}

function isActiveStatus(status: TransferStatus) {
  return status === "uploading" || status === "paused" || status === "processing";
}

function statusLabel(status: TransferStatus, t: (key: string) => string) {
  const keyMap: Record<TransferStatus, string> = {
    uploading: "transfer.status.uploading",
    paused: "transfer.status.paused",
    processing: "transfer.status.processing",
    done: "transfer.status.done",
    failed: "transfer.status.failed",
    cancelled: "transfer.status.cancelled",
    expired: "transfer.status.expired",
  };
  return t(keyMap[status]);
}
