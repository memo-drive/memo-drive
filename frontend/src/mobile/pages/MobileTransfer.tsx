import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { message } from "../../components/base";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import { useTransferStore } from "../../stores/transferStore";
import type { TransferTask } from "../../stores/transferStore";
import { MobileTransferView } from "./MobileTransferView";
import styles from "./MobilePlaceholder.module.css";

export function MobileTransferPage() {
  const { t } = useTranslation();
  const resumeInputRef = useRef<HTMLInputElement | null>(null);
  const resumeTargetRef = useRef<TransferTask | null>(null);
  const { tasks, loadingHistory, loadSessions, removeTask, clearDone } = useTransferStore();
  const { pause, resume, cancel } = useChunkedUpload(() => undefined);

  useEffect(() => {
    void loadSessions();
  }, [loadSessions]);

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

  async function handleClearDone() {
    try {
      await clearDone();
      message.success(t("transfer.clearSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("transfer.clearFailed"));
    }
  }

  return (
    <section className={styles.page}>
      <h1>{t("mobile.transfer.title")}</h1>
      <p>{t("mobile.transfer.subtitle")}</p>
      <input
        ref={resumeInputRef}
        type="file"
        hidden
        onChange={(event) => void handleResumeFile(event.target.files)}
      />
      <MobileTransferView
        tasks={tasks}
        loading={loadingHistory}
        onPause={pause}
        onResume={handleResume}
        onCancel={(id) => void handleCancel(id)}
        onRemove={(id) => void handleRemove(id)}
        onClearDone={() => void handleClearDone()}
      />
    </section>
  );
}
