import { Link } from "react-router-dom";
import { useTranslation } from "react-i18next";
import type { StorageUsage } from "../../types";
import { formatBytes } from "../../utils/formatBytes";
import { storageUsagePresentation } from "../../utils/storageUsage";
import styles from "./StorageUsagePanel.module.css";

interface StorageUsagePanelProps {
  usage: StorageUsage;
}

export function StorageUsagePanel({ usage }: StorageUsagePanelProps) {
  const { t } = useTranslation();
  const presentation = storageUsagePresentation(usage);
  const categories = [
    {
      icon: "folder",
      label: t("storageUsage.active"),
      value: usage.active_bytes,
    },
    {
      icon: "delete",
      label: t("storageUsage.trash"),
      value: usage.trash_bytes,
    },
    {
      icon: "hourglass_top",
      label: t("storageUsage.temp"),
      value: usage.temp_bytes,
    },
    {
      icon: "hard_drive",
      label: t("storageUsage.diskAvailable"),
      value: usage.filesystem_available_bytes,
    },
  ];

  return (
    <section className={styles.panel} aria-labelledby="storage-usage-title">
      <div className={styles.header}>
        <div>
          <h3 id="storage-usage-title">{t("storageUsage.title")}</h3>
          <p>{t("storageUsage.description")}</p>
        </div>
        {usage.quota_bytes > 0 ? (
          <span className={styles.quotaBadge}>
            {t("storageUsage.quota", {
              value: formatBytes(usage.quota_bytes),
            })}
          </span>
        ) : null}
      </div>

      <div className={styles.available}>
        <span>{t("storageUsage.uploadAvailable")}</span>
        <strong>{formatBytes(usage.upload_available_bytes)}</strong>
      </div>
      {presentation.uploadCapacity > 0 ? (
        <div
          className={styles.track}
          role="progressbar"
          aria-label={t("storageUsage.title")}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(presentation.usedPercent)}
        >
          <span style={{ width: `${presentation.usedPercent}%` }} />
        </div>
      ) : null}

      <div className={styles.categories}>
        {categories.map((category) => (
          <div key={category.label} className={styles.category}>
            <span className="material-symbols-outlined" aria-hidden>
              {category.icon}
            </span>
            <div>
              <span>{category.label}</span>
              <strong>{formatBytes(category.value)}</strong>
            </div>
          </div>
        ))}
      </div>

      {presentation.low ? (
        <div className={styles.warning} role="alert">
          <span className="material-symbols-outlined" aria-hidden>
            warning
          </span>
          <div>
            <strong>{t("storageUsage.lowTitle")}</strong>
            <p>{t("storageUsage.lowBody")}</p>
          </div>
          <Link to="/trash">{t("storageUsage.cleanTrash")}</Link>
        </div>
      ) : null}
    </section>
  );
}
