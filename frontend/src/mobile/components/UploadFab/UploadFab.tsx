import { useEffect, useId, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { browserSupportsDirectorySelection } from "../../../workflows/directoryUploadWorkflow";
import styles from "./UploadFab.module.css";

interface UploadFabProps {
  onFiles?: (files: FileList | null) => void;
  onDirectoryFiles?: (files: FileList | null) => void;
}

export function UploadFab({ onFiles, onDirectoryFiles }: UploadFabProps) {
  const { t } = useTranslation();
  const inputId = useId();
  const directoryInputId = useId();
  const directoryInputRef = useRef<HTMLInputElement | null>(null);
  const [directorySupported, setDirectorySupported] = useState(false);

  useEffect(() => {
    if (directoryInputRef.current) {
      setDirectorySupported(browserSupportsDirectorySelection(directoryInputRef.current));
    }
  }, []);

  return (
    <div className={styles.wrap}>
      <input
        ref={directoryInputRef}
        id={directoryInputId}
        className={styles.input}
        type="file"
        multiple
        {...({ webkitdirectory: "" } as Record<string, string>)}
        onChange={(event) => {
          onDirectoryFiles?.(event.currentTarget.files);
          event.currentTarget.value = "";
        }}
      />
      <input
        id={inputId}
        className={styles.input}
        type="file"
        multiple
        onChange={(event) => {
          onFiles?.(event.currentTarget.files);
          event.currentTarget.value = "";
        }}
      />
      <label className={styles.button} htmlFor={inputId} aria-label={t("mobile.files.uploadFab")}>
        <span className="material-symbols-outlined" aria-hidden>
          upload_file
        </span>
      </label>
      {directorySupported ? (
        <label
          className={`${styles.button} ${styles.directoryButton}`}
          htmlFor={directoryInputId}
          aria-label={t("drive.uploadFolder")}
        >
          <span className="material-symbols-outlined" aria-hidden>
            drive_folder_upload
          </span>
        </label>
      ) : null}
    </div>
  );
}
