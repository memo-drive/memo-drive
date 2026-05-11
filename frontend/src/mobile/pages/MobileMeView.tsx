import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { StorageUsage } from "../../types";
import { formatBytes } from "../../utils/formatBytes";
import styles from "./MobileMeView.module.css";

interface MobileMeViewProps {
  storageUsage?: StorageUsage | null;
  currentLanguage?: "zh-CN" | "en";
  onLanguageChange?: (language: "zh-CN" | "en") => void;
  onLogout?: () => void;
}

export function MobileMeView({
  storageUsage,
  currentLanguage = "zh-CN",
  onLanguageChange,
  onLogout,
}: MobileMeViewProps) {
  const { t } = useTranslation();
  const languageOptions: { value: "zh-CN" | "en"; label: string }[] = [
    { value: "zh-CN", label: "中文" },
    { value: "en", label: "English" },
  ];

  return (
    <div className={styles.wrap}>
      {storageUsage ? (
        <section className={styles.storageCard}>
          <div>
            <span className="material-symbols-outlined" aria-hidden>
              database
            </span>
            <strong>{t("layout.storage")}</strong>
          </div>
          <p>
            {t("layout.storageUsage", {
              used: formatBytes(storageUsage.used_bytes),
              total: formatBytes(storageUsage.total_bytes),
            })}
          </p>
          {storageUsage.total_bytes > 0 ? (
            <div className={styles.track}>
              <span
                style={{
                  width: `${Math.min(
                    100,
                    Math.max(0, (storageUsage.used_bytes / storageUsage.total_bytes) * 100),
                  )}%`,
                }}
              />
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
    </div>
  );
}
