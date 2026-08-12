import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button, message } from "../../components/base";
import { PipelineTaskPanel } from "../../components/PipelineTask/PipelineTaskPanel";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import {
  directoryTransferSummaries,
  isActiveTransferStatus,
  transferStatusLabelKey,
} from "../../stores/transferProjection";
import { useTransferStore } from "../../stores/transferStore";
import type { TransferStatus, TransferTask } from "../../stores/transferStore";
import styles from "./index.module.css";

type TabKey = "active" | "history" | "processing";

const SIZE_UNITS = ["B", "KB", "MB", "GB", "TB"];

function formatSize(bytes: number): string {
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < SIZE_UNITS.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${SIZE_UNITS[i]}`;
}

function formatDuration(seconds: number, t: (key: string, options?: Record<string, unknown>) => string): string {
  if (!Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return t("transfer.etaSeconds", { seconds: Math.ceil(seconds) });
  if (seconds < 3600) return t("transfer.etaMinutes", { minutes: Math.ceil(seconds / 60) });
  return t("transfer.etaHours", { hours: Math.ceil(seconds / 3600) });
}

function formatSpeed(bytesPerSecond: number): string {
  if (!bytesPerSecond || bytesPerSecond <= 0) return "";
  return `${formatSize(bytesPerSecond)}/s`;
}

function formatTime(timestamp: number): string {
  const d = new Date(timestamp);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

function statusClass(status: TransferStatus): string {
  if (status === "preparing" || status === "uploading" || status === "processing") return styles.statusRunning;
  if (status === "done") return styles.statusSuccess;
  if (status === "paused") return styles.statusPaused;
  if (status === "cancelled" || status === "expired") return styles.statusMuted;
  return styles.statusFailed;
}

function uploadedBytes(task: TransferTask) {
  return Math.min(task.uploadedChunks.length * task.chunkSize, task.fileSize);
}

function remainingText(task: TransferTask, t: (key: string, options?: Record<string, unknown>) => string) {
  if (task.status !== "uploading" || task.speed <= 0) return "";
  return formatDuration((task.fileSize - uploadedBytes(task)) / task.speed, t);
}

function TransferCard({
  item,
  onPause,
  onResume,
  onRetryConflict,
  onCancel,
  onRemove,
}: {
  item: TransferTask;
  onPause: (id: string) => void;
  onResume: (task: TransferTask) => void;
  onRetryConflict: (
    task: TransferTask,
    policy: "rename" | "replace",
  ) => void;
  onCancel: (id: string) => void;
  onRemove: (id: string) => void;
}) {
  const { t } = useTranslation();
  const speed = formatSpeed(item.speed);
  const remaining = remainingText(item, t);
  const showProgress = item.status !== "done";
  const requestedName = item.requestedName ?? item.fileName;
  const resolvedName = item.resolvedName ?? item.fileName;
  return (
    <div className={styles.transferItem}>
      <div className={styles.itemIcon}>
        <span className="material-symbols-outlined">upload</span>
      </div>
      <div className={styles.itemInfo}>
        <div className={styles.itemName}>{resolvedName}</div>
        {requestedName !== resolvedName && (
          <div className={styles.resolvedName}>
            {t("transfer.resolvedName", {
              requested: requestedName,
              resolved: resolvedName,
            })}
          </div>
        )}
        <div className={styles.itemMeta}>
          <span>{t("transfer.direction")}</span>
          <span>{formatSize(item.fileSize)}</span>
          {speed && item.status === "uploading" && <span>{speed}</span>}
          <span>{formatTime(item.createdAt)}</span>
          {item.expiresAt && item.status === "paused" && (
            <span>{t("transfer.sessionExpiry", { time: new Date(item.expiresAt).toLocaleString() })}</span>
          )}
        </div>
        {item.error && <div className={styles.itemError}>{item.error}</div>}
      </div>
      <div className={styles.itemRight}>
        {showProgress && (
          <div className={styles.progressWrap}>
            <div className={styles.progressBar}>
              <div
                className={styles.progressFill}
                style={{ width: `${Math.max(0, Math.min(100, item.percent))}%` }}
              />
            </div>
            <div className={styles.progressPercent}>
              {remaining && `${remaining} · `}
              {item.percent}%
            </div>
          </div>
        )}
        <span className={`${styles.statusBadge} ${statusClass(item.status)}`}>
          {t(transferStatusLabelKey(item.status))}
        </span>
        <div className={styles.actions}>
          {item.status === "uploading" && (
            <>
              <button className={styles.actionBtn} onClick={() => onPause(item.id)}>
                {t("transfer.actionPause")}
              </button>
              <button className={styles.actionBtn} onClick={() => onCancel(item.id)}>
                {t("transfer.actionCancel")}
              </button>
            </>
          )}
          {item.status === "paused" && (
            <>
              <button className={styles.actionBtn} onClick={() => onResume(item)}>
                {t("transfer.actionResume")}
              </button>
              <button className={styles.actionBtn} onClick={() => onCancel(item.id)}>
                {t("transfer.actionCancel")}
              </button>
            </>
          )}
          {item.status === "failed" && item.conflictRetryable && (
            <>
              <button
                className={styles.actionBtn}
                onClick={() => onRetryConflict(item, "rename")}
              >
                {t("transfer.retryKeepBoth")}
              </button>
              <button
                className={styles.actionBtn}
                onClick={() => onRetryConflict(item, "replace")}
              >
                {t("transfer.retryReplace")}
              </button>
            </>
          )}
          {!isActiveTransferStatus(item.status) && (
            <button className={styles.actionBtn} onClick={() => onRemove(item.id)}>
              {t("transfer.actionRemove")}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}

export function TransferPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<TabKey>("active");
  const resumeInputRef = useRef<HTMLInputElement | null>(null);
  const resumeTargetRef = useRef<TransferTask | null>(null);
  const conflictInputRef = useRef<HTMLInputElement | null>(null);
  const conflictTargetRef = useRef<{
    task: TransferTask;
    policy: "rename" | "replace";
  } | null>(null);
  const { tasks, loadingHistory, loadSessions, removeTask, clearDone } =
    useTransferStore();
  const { pause, resume, retryConflict, cancel } = useChunkedUpload(() => undefined);

  useEffect(() => {
    void loadSessions();
  }, [loadSessions]);

  const activeList = tasks.filter((task) => isActiveTransferStatus(task.status));
  const historyList = tasks.filter((task) => !isActiveTransferStatus(task.status));
  const list = tab === "active" ? activeList : historyList;
  const directorySummaries = directoryTransferSummaries(tasks);

  function handleResume(task: TransferTask) {
    if (task.file) {
      void resume(task.id).catch((err) => {
        message.error(err instanceof Error ? err.message : t("transfer.resumeFailed"));
      });
      return;
    }
    resumeTargetRef.current = task;
    resumeInputRef.current?.click();
  }

  async function handleResumeFile(files: FileList | null) {
    const task = resumeTargetRef.current;
    resumeTargetRef.current = null;
    if (!task || !files || files.length === 0) return;
    try {
      await resume(task.id, files[0]);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("transfer.resumeFailed"));
    } finally {
      if (resumeInputRef.current) {
        resumeInputRef.current.value = "";
      }
    }
  }

  function handleConflictRetry(
    task: TransferTask,
    policy: "rename" | "replace",
  ) {
    if (task.file) {
      void retryConflict(task, policy).catch((err) => {
        message.error(err instanceof Error ? err.message : t("transfer.retryFailed"));
      });
      return;
    }
    conflictTargetRef.current = { task, policy };
    conflictInputRef.current?.click();
  }

  async function handleConflictFile(files: FileList | null) {
    const target = conflictTargetRef.current;
    conflictTargetRef.current = null;
    if (!target || !files || files.length === 0) return;
    try {
      await retryConflict(target.task, target.policy, files[0]);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("transfer.retryFailed"));
    } finally {
      if (conflictInputRef.current) {
        conflictInputRef.current.value = "";
      }
    }
  }

  async function handleCancel(id: string) {
    await cancel(id);
    message.success(t("transfer.cancelSuccess"));
  }

  async function handleRemove(id: string) {
    try {
      await removeTask(id);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("transfer.removeFailed"));
    }
  }

  async function handleClear() {
    try {
      await clearDone();
      message.success(t("transfer.clearSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("transfer.clearFailed"));
    }
  }

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          <h2>{t("transfer.title")}</h2>
          <p>{t("transfer.subtitle")}</p>
        </div>
        {tab === "history" && historyList.length > 0 && (
          <Button variant="secondary" onClick={handleClear}>
            {t("transfer.clearAll")}
          </Button>
        )}
      </div>

      <input
        ref={resumeInputRef}
        type="file"
        hidden
        onChange={(event) => handleResumeFile(event.target.files)}
      />
      <input
        ref={conflictInputRef}
        type="file"
        hidden
        onChange={(event) => void handleConflictFile(event.target.files)}
      />

      {tab !== "processing" && directorySummaries.length > 0 ? (
        <div className={styles.directoryGroups}>
          {directorySummaries.map((group) => (
            <section key={group.batchId} className={styles.directoryGroup}>
              <div>
                <strong>{group.name}</strong>
                <span>
                  {t("transfer.directoryFiles", {
                    completed: group.completedCount,
                    total: group.fileCount,
                  })}
                </span>
              </div>
              <div className={styles.progressWrap}>
                <div className={styles.progressBar}>
                  <div className={styles.progressFill} style={{ width: `${group.percent}%` }} />
                </div>
                <div className={styles.progressPercent}>{group.percent}%</div>
              </div>
            </section>
          ))}
        </div>
      ) : null}

      <div className={styles.tabs}>
        <button
          className={`${styles.tab} ${tab === "active" ? styles.tabActive : ""}`}
          aria-pressed={tab === "active"}
          onClick={() => setTab("active")}
        >
          {t("transfer.tabActive")}
          {activeList.length > 0 && ` (${activeList.length})`}
        </button>
        <button
          className={`${styles.tab} ${tab === "history" ? styles.tabActive : ""}`}
          aria-pressed={tab === "history"}
          onClick={() => setTab("history")}
        >
          {t("transfer.tabHistory")}
          {historyList.length > 0 && ` (${historyList.length})`}
        </button>
        <button
          className={`${styles.tab} ${tab === "processing" ? styles.tabActive : ""}`}
          aria-pressed={tab === "processing"}
          onClick={() => setTab("processing")}
        >
          {t("transfer.tabProcessing")}
        </button>
      </div>

      <section className={styles.card}>
        {tab === "processing" ? (
          <div className={styles.pipelinePanel}>
            <PipelineTaskPanel />
          </div>
        ) : loadingHistory && list.length === 0 ? (
          <div className={styles.empty}>
            <span className={`material-symbols-outlined ${styles.emptyIcon}`}>
              sync
            </span>
            <p className={styles.emptyText}>{t("transfer.syncing")}</p>
          </div>
        ) : list.length === 0 ? (
          <div className={styles.empty}>
            <span className={`material-symbols-outlined ${styles.emptyIcon}`}>
              {tab === "active" ? "cloud_upload" : "history"}
            </span>
            <p className={styles.emptyText}>
              {tab === "active" ? t("transfer.emptyActive") : t("transfer.emptyHistory")}
            </p>
          </div>
        ) : (
          list.map((item) => (
            <TransferCard
              key={item.id}
              item={item}
              onPause={pause}
              onResume={handleResume}
              onRetryConflict={handleConflictRetry}
              onCancel={handleCancel}
              onRemove={handleRemove}
            />
          ))
        )}
      </section>
    </div>
  );
}
