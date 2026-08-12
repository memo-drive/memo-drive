import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { httpClient } from "../../api/client";
import { getStorageUsage } from "../../api/fileApi";
import { Button, Segment } from "../../components/base";
import type { SegmentOption } from "../../components/base";
import { StorageUsagePanel } from "../../components/StorageUsage/StorageUsagePanel";
import type { StorageUsage } from "../../types";

export function SettingsPage() {
	const { t, i18n } = useTranslation();
	const navigate = useNavigate();
  const [storageUsage, setStorageUsage] = useState<StorageUsage | null>(null);
  const [storageError, setStorageError] = useState(false);

  const current = i18n.language?.startsWith("zh") ? "zh-CN" : "en";

	const languageOptions: SegmentOption[] = [
    { value: "zh-CN", label: "中文" },
    { value: "en", label: "English" },
	];

	const handleLogoutAll = async () => {
		await httpClient.logout("all");
		navigate("/login", { replace: true });
	};

  useEffect(() => {
    let cancelled = false;
    getStorageUsage()
      .then((usage) => {
        if (cancelled) return;
        setStorageUsage(usage);
        setStorageError(false);
      })
      .catch(() => {
        if (cancelled) return;
        setStorageUsage(null);
        setStorageError(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return (
    <div className="flex flex-col h-full overflow-auto">
      {/* Page header */}
      <div className="mb-6">
        <h2 className="text-[1.63rem] font-bold text-zinc-900">
          {t("settings.title")}
        </h2>
      </div>

      <section className="mb-6">
        {storageUsage ? <StorageUsagePanel usage={storageUsage} /> : null}
        {storageError ? (
          <p className="text-sm text-red-700" role="alert">
            {t("storageUsage.loadFailed")}
          </p>
        ) : null}
      </section>

      {/* Language section */}
      <section>
        <h3 className="text-sm font-semibold text-zinc-600 mb-2">
          {t("settings.language")}
        </h3>
        <Segment
          options={languageOptions}
          value={current}
          onChange={(val) => i18n.changeLanguage(val)}
        />
      </section>

	  <section className="mt-8">
		<h3 className="text-sm font-semibold text-zinc-600 mb-2">
		  {t("auth.sessions")}
		</h3>
		<Button variant="ghost" className="!text-red-600" onClick={handleLogoutAll}>
		  {t("auth.logoutAll")}
		</Button>
	  </section>
    </div>
  );
}
