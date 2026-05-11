import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { clearToken } from "../../api/client";
import { getStorageUsage } from "../../api/fileApi";
import type { StorageUsage } from "../../types";
import { MobileMeView } from "./MobileMeView";
import styles from "./MobilePlaceholder.module.css";

export function MobileMePage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const currentLanguage = i18n.language?.startsWith("zh") ? "zh-CN" : "en";

  useEffect(() => {
    let cancelled = false;
    getStorageUsage()
      .then((usage) => {
        if (!cancelled) setStorageUsage(usage);
      })
      .catch(() => {
        if (!cancelled) setStorageUsage(null);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  function handleLogout() {
    clearToken();
    navigate("/login", { replace: true });
  }

  return (
    <section className={styles.page}>
      <h1>{t("mobile.me.title")}</h1>
      <p>{t("mobile.me.subtitle")}</p>
      <MobileMeView
        storageUsage={storageUsage}
        currentLanguage={currentLanguage}
        onLanguageChange={(language) => void i18n.changeLanguage(language)}
        onLogout={handleLogout}
      />
    </section>
  );
}
