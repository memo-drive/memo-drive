import { useTranslation } from "react-i18next";
import { displayName } from "../../components/FileManager/TrashList";
import type { DriveFile } from "../../types";
import { MobileConfirmPrompt } from "../components/ConfirmPrompt/MobileConfirmPrompt";
import styles from "./MobileTrashView.module.css";

interface MobileTrashViewProps {
  files: DriveFile[];
  loading: boolean;
  actionId: string;
  emptying: boolean;
  purgeConfirmFile?: DriveFile | null;
  emptyConfirmOpen?: boolean;
  onRestore: (file: DriveFile) => void;
  onPurge: (file: DriveFile) => void;
  onEmpty: () => void;
  onCancelPurge?: () => void;
  onConfirmPurge?: () => void;
  onCancelEmpty?: () => void;
  onConfirmEmpty?: () => void;
}

export function MobileTrashView({
  files,
  loading,
  actionId,
  emptying,
  purgeConfirmFile = null,
  emptyConfirmOpen = false,
  onRestore,
  onPurge,
  onEmpty,
  onCancelPurge,
  onConfirmPurge,
  onCancelEmpty,
  onConfirmEmpty,
}: MobileTrashViewProps) {
  const { t } = useTranslation();

  if (loading && files.length === 0) {
    return <div className={styles.state}>{t("trash.loading")}</div>;
  }

  if (files.length === 0) {
    return <div className={styles.state}>{t("trash.empty")}</div>;
  }

  return (
    <div className={styles.wrap}>
      <button
        className={styles.emptyButton}
        type="button"
        disabled={emptying}
        onClick={onEmpty}
      >
        {t("trash.emptyTrash")}
      </button>
      <div className={styles.list}>
        {files.map((file) => (
          <article key={file.id} className={styles.card}>
            <div className={styles.header}>
              <span className="material-symbols-outlined" aria-hidden>
                {file.is_dir ? "folder" : "description"}
              </span>
              <div>
                <h2>{displayName(file)}</h2>
                <p>{file.original_path ?? file.path}</p>
              </div>
            </div>
            <div className={styles.actions}>
              <button
                type="button"
                disabled={actionId === `restore:${file.id}`}
                onClick={() => onRestore(file)}
              >
                {t("common.restore")}
              </button>
              <button
                className={styles.danger}
                type="button"
                disabled={actionId === `purge:${file.id}`}
                onClick={() => onPurge(file)}
              >
                {t("common.purge")}
              </button>
            </div>
          </article>
        ))}
      </div>
      <MobileConfirmPrompt
        open={Boolean(purgeConfirmFile)}
        title={t("trash.purgeConfirmTitle")}
        description={
          purgeConfirmFile
            ? t("trash.purgeConfirmBody", { name: displayName(purgeConfirmFile) })
            : ""
        }
        confirmText={t("common.purge")}
        tone="danger"
        busy={Boolean(purgeConfirmFile && actionId === `purge:${purgeConfirmFile.id}`)}
        onCancel={() => onCancelPurge?.()}
        onConfirm={() => onConfirmPurge?.()}
      />
      <MobileConfirmPrompt
        open={emptyConfirmOpen}
        title={t("trash.emptyConfirmTitle")}
        description={t("trash.emptyConfirmBody", { count: files.length })}
        confirmText={t("trash.emptyTrash")}
        tone="danger"
        busy={emptying}
        onCancel={() => onCancelEmpty?.()}
        onConfirm={() => onConfirmEmpty?.()}
      />
    </div>
  );
}
