import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  emptyTrash,
  listTrash,
  purgeFile,
  restoreFile,
} from "../../api/fileApi";
import { message } from "../../components/base";
import { displayName } from "../../components/FileManager/TrashList";
import type { DriveFile } from "../../types";
import {
  runMobileTrashEmpty,
  runMobileTrashPurge,
  runMobileTrashRestore,
} from "./mobileTrashActions";
import { MobileTrashView } from "./MobileTrashView";
import styles from "./MobilePlaceholder.module.css";

export function MobileTrashPage() {
  const { t } = useTranslation();
  const [files, setFiles] = useState<DriveFile[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionId, setActionId] = useState("");
  const [emptying, setEmptying] = useState(false);
  const [purgeConfirmFile, setPurgeConfirmFile] = useState<DriveFile | null>(null);
  const [emptyConfirmOpen, setEmptyConfirmOpen] = useState(false);

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
  }, [t]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function handleRestore(file: DriveFile) {
    const name = displayName(file);
    setActionId(`restore:${file.id}`);
    try {
      await runMobileTrashRestore(file, {
        restore: restoreFile,
        refresh,
      });
      message.success(t("trash.restoreSuccess", { name }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.restoreFailed"));
    } finally {
      setActionId("");
    }
  }

  function requestPurge(file: DriveFile) {
    setPurgeConfirmFile(file);
  }

  async function confirmPurge() {
    const file = purgeConfirmFile;
    if (!file) return;
    const name = displayName(file);
    setActionId(`purge:${file.id}`);
    try {
      await runMobileTrashPurge(file, {
        purge: purgeFile,
        refresh,
      });
      setPurgeConfirmFile(null);
      message.success(t("trash.purgeSuccess", { name }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.purgeFailed"));
    } finally {
      setActionId("");
    }
  }

  function requestEmpty() {
    setEmptyConfirmOpen(true);
  }

  async function confirmEmpty() {
    setEmptying(true);
    try {
      const response = await runMobileTrashEmpty({
        empty: emptyTrash,
        refresh,
      });
      setEmptyConfirmOpen(false);
      message.success(t("trash.emptySuccess", { count: response.purged }));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("trash.emptyFailed"));
    } finally {
      setEmptying(false);
    }
  }

  return (
    <section className={styles.page}>
      <h1>{t("mobile.trash.title")}</h1>
      <p>{t("mobile.trash.subtitle")}</p>
      <MobileTrashView
        files={files}
        loading={loading}
        actionId={actionId}
        emptying={emptying}
        purgeConfirmFile={purgeConfirmFile}
        emptyConfirmOpen={emptyConfirmOpen}
        onRestore={handleRestore}
        onPurge={requestPurge}
        onEmpty={requestEmpty}
        onCancelPurge={() => setPurgeConfirmFile(null)}
        onConfirmPurge={() => void confirmPurge()}
        onCancelEmpty={() => setEmptyConfirmOpen(false)}
        onConfirmEmpty={() => void confirmEmpty()}
      />
    </section>
  );
}
