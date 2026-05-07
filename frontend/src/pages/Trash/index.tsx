import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  emptyTrash,
  listTrash,
  purgeFile,
  restoreFile,
} from "../../api/fileApi";
import { Button, message, Modal } from "../../components/base";
import {
  displayName,
  TrashList,
} from "../../components/FileManager/TrashList";
import type { DriveFile } from "../../types";
import styles from "./index.module.css";

export function TrashPage() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionId, setActionId] = useState("");
  const [purgeTarget, setPurgeTarget] = useState<DriveFile | null>(null);
  const [confirmingEmpty, setConfirmingEmpty] = useState(false);
  const [emptying, setEmptying] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const response = await listTrash();
      setFiles(response.files ?? []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.loadError"));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function handleRestore(file: DriveFile) {
    const name = displayName(file);
    setActionId(`restore:${file.id}`);
    try {
      await restoreFile(file.id);
      message.success(file.is_dir ? t("trash.restoreSuccess", { name }) : `${name} 已恢复`);
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.restoreFailed"));
    } finally {
      setActionId("");
    }
  }

  async function handlePurgeConfirm() {
    if (!purgeTarget) return;
    const name = displayName(purgeTarget);
    setActionId(`purge:${purgeTarget.id}`);
    try {
      await purgeFile(purgeTarget.id);
      setPurgeTarget(null);
      message.success(t("trash.purgeSuccess", { name }));
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.purgeFailed"));
    } finally {
      setActionId("");
    }
  }

  async function handleEmptyConfirm() {
    setEmptying(true);
    try {
      const response = await emptyTrash();
      setConfirmingEmpty(false);
      message.success(t("trash.emptySuccess", { count: response.purged }));
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.emptyFailed"));
    } finally {
      setEmptying(false);
    }
  }

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          <h2>{t("trash.title")}</h2>
          <p>{t("trash.subtitle")}</p>
        </div>
        <Button
          variant="danger"
          disabled={files.length === 0 || loading}
          onClick={() => setConfirmingEmpty(true)}
        >
          {t("trash.emptyTrash")}
        </Button>
      </div>

      <section className={styles.card}>
        <TrashList
          files={files}
          loading={loading}
          actionId={actionId}
          onRestore={handleRestore}
          onPurge={setPurgeTarget}
        />
      </section>

      <Modal
        open={!!purgeTarget}
        onClose={() => setPurgeTarget(null)}
        title={t("trash.purgeConfirmTitle")}
        footer={
          <>
            <Button variant="secondary" onClick={() => setPurgeTarget(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              onClick={handlePurgeConfirm}
              loading={actionId === `purge:${purgeTarget?.id}`}
            >
              {t("common.purge")}
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          {t("trash.purgeConfirmBody", { name: purgeTarget ? displayName(purgeTarget) : "" })}
        </p>
      </Modal>

      <Modal
        open={confirmingEmpty}
        onClose={() => setConfirmingEmpty(false)}
        title={t("trash.emptyConfirmTitle")}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirmingEmpty(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              onClick={handleEmptyConfirm}
              loading={emptying}
              disabled={files.length === 0}
            >
              {t("trash.emptyTrash")}
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          {t("trash.emptyConfirmBody", { count: files.length })}
        </p>
      </Modal>
    </div>
  );
}
