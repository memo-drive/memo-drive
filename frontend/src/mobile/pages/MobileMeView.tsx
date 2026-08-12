import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { StorageUsage } from "../../types";
import { formatBytes } from "../../utils/formatBytes";
import { storageUsagePresentation } from "../../utils/storageUsage";
import styles from "./MobileMeView.module.css";

interface MobileMeViewProps {
  storageUsage?: StorageUsage | null;
  currentLanguage?: "zh-CN" | "en";
  onLanguageChange?: (language: "zh-CN" | "en") => void;
  onLogout?: () => void;
	onLogoutAll?: () => void;
}

export function MobileMeView({
  storageUsage,
  currentLanguage = "zh-CN",
  onLanguageChange,
  onLogout,
	onLogoutAll,
}: MobileMeViewProps) {
  const { t } = useTranslation();
  const languageOptions: { value: "zh-CN" | "en"; label: string }[] = [
    { value: "zh-CN", label: "中文" },
    { value: "en", label: "English" },
  ];
  const storagePresentation = storageUsage
    ? storageUsagePresentation(storageUsage)
    : null;

  return (
    <div className={styles.wrap}>
      {storageUsage ? (
        <section className={styles.storageCard}>
          <div className={styles.storageTitle}>
            <span className="material-symbols-outlined" aria-hidden>
              hard_drive
            </span>
            <strong>{t("storageUsage.title")}</strong>
          </div>
          <div className={styles.available}>
            <span>{t("storageUsage.uploadAvailable")}</span>
            <strong>{formatBytes(storageUsage.upload_available_bytes)}</strong>
          </div>
          {storagePresentation && storagePresentation.uploadCapacity > 0 ? (
            <div
              className={styles.track}
              role="progressbar"
              aria-label={t("storageUsage.title")}
              aria-valuemin={0}
              aria-valuemax={100}
              aria-valuenow={Math.round(storagePresentation.usedPercent)}
            >
              <span
                style={{
                  width: `${storagePresentation.usedPercent}%`,
                }}
              />
            </div>
          ) : null}
          <div className={styles.storageGrid}>
            <div>
              <span>{t("storageUsage.active")}</span>
              <strong>{formatBytes(storageUsage.active_bytes)}</strong>
            </div>
            <div>
              <span>{t("storageUsage.trash")}</span>
              <strong>{formatBytes(storageUsage.trash_bytes)}</strong>
            </div>
            <div>
              <span>{t("storageUsage.temp")}</span>
              <strong>{formatBytes(storageUsage.temp_bytes)}</strong>
            </div>
            <div>
              <span>{t("storageUsage.diskAvailable")}</span>
              <strong>{formatBytes(storageUsage.filesystem_available_bytes)}</strong>
            </div>
          </div>
          {storagePresentation?.low ? (
            <div className={styles.storageWarning} role="alert">
              <span className="material-symbols-outlined" aria-hidden>
                warning
              </span>
              <div>
                <strong>{t("storageUsage.lowTitle")}</strong>
                <p>{t("storageUsage.lowBody")}</p>
                <Link to="/m/trash">{t("storageUsage.cleanTrash")}</Link>
              </div>
            </div>
          ) : null}
        </section>
      ) : null}

      <section className={styles.panel}>
        <div className={styles.panelTitle}>
          <span className="material-symbols-outlined" aria-hidden>
            language
          </span>
          <strong>{t("settings.language")}</strong>
        </div>
        <div className={styles.languageGroup} role="group" aria-label={t("settings.language")}>
          {languageOptions.map((option) => (
            <button
              key={option.value}
              className={`${styles.languageButton} ${
                option.value === currentLanguage ? styles.languageButtonActive : ""
              }`}
              type="button"
              aria-pressed={option.value === currentLanguage}
              onClick={() => onLanguageChange?.(option.value)}
            >
              {option.label}
            </button>
          ))}
        </div>
      </section>

      <div className={styles.linkList}>
        <Link to="/m/trash">
          <span className="material-symbols-outlined" aria-hidden>
            delete
          </span>
          <span>{t("mobile.me.trash")}</span>
          <span className="material-symbols-outlined" aria-hidden>
            chevron_right
          </span>
        </Link>
      </div>

      <button className={styles.logoutButton} type="button" onClick={onLogout}>
        <span className="material-symbols-outlined" aria-hidden>
          logout
        </span>
        <span>{t("layout.logout")}</span>
      </button>
	  <button className={styles.logoutButton} type="button" onClick={onLogoutAll}>
		<span className="material-symbols-outlined" aria-hidden>
			devices
		</span>
		<span>{t("auth.logoutAll")}</span>
	  </button>
    </div>
  );
}
