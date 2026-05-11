import { useId } from "react";
import { useTranslation } from "react-i18next";
import styles from "./UploadFab.module.css";

interface UploadFabProps {
  onFiles?: (files: FileList | null) => void;
}

export function UploadFab({ onFiles }: UploadFabProps) {
  const { t } = useTranslation();
  const inputId = useId();

  return (
    <div className={styles.wrap}>
      <input
        id={inputId}
        className={styles.input}
        type="file"
        multiple
        onChange={(event) => onFiles?.(event.currentTarget.files)}
      />
      <label className={styles.button} htmlFor={inputId} aria-label={t("mobile.files.uploadFab")}>
        <span className="material-symbols-outlined" aria-hidden>
          upload_file
        </span>
      </label>
    </div>
  );
}
