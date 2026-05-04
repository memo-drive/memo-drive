import { useCallback, useEffect, useState } from "react";
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
      message.error(err instanceof Error ? err.message : "加载回收站失败");
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
      message.success(file.is_dir ? `${name} 及其中内容已恢复` : `${name} 已恢复`);
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "恢复失败");
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
      message.success(`${name} 已永久删除`);
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "永久删除失败");
    } finally {
      setActionId("");
    }
  }

  async function handleEmptyConfirm() {
    setEmptying(true);
    try {
      const response = await emptyTrash();
      setConfirmingEmpty(false);
      message.success(`已清空回收站（${response.purged} 项）`);
      await refresh();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "清空回收站失败");
    } finally {
      setEmptying(false);
    }
  }

  return (
    <div className={styles.pageWrapper}>
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          <h2>回收站</h2>
          <p>回收站内的文件会保留 30 天，过期后自动清理。</p>
        </div>
        <Button
          variant="danger"
          disabled={files.length === 0 || loading}
          onClick={() => setConfirmingEmpty(true)}
        >
          清空回收站
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
        title="永久删除"
        footer={
          <>
            <Button variant="secondary" onClick={() => setPurgeTarget(null)}>
              取消
            </Button>
            <Button
              variant="danger"
              onClick={handlePurgeConfirm}
              loading={actionId === `purge:${purgeTarget?.id}`}
            >
              永久删除
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          确定要永久删除{" "}
          <span className="font-semibold text-notion-black">
            {purgeTarget ? displayName(purgeTarget) : ""}
          </span>{" "}
          吗？此操作不可撤销。
        </p>
      </Modal>

      <Modal
        open={confirmingEmpty}
        onClose={() => setConfirmingEmpty(false)}
        title="清空回收站"
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirmingEmpty(false)}>
              取消
            </Button>
            <Button
              variant="danger"
              onClick={handleEmptyConfirm}
              loading={emptying}
              disabled={files.length === 0}
            >
              清空回收站
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          确定要永久删除回收站中的全部 {files.length} 项吗？此操作不可撤销。
        </p>
      </Modal>
    </div>
  );
}
