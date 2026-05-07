import { useTranslation } from "react-i18next";
import { Segment } from "../../components/base";
import type { SegmentOption } from "../../components/base";

export function SettingsPage() {
  const { t, i18n } = useTranslation();

  const current = i18n.language?.startsWith("zh") ? "zh-CN" : "en";

  const languageOptions: SegmentOption[] = [
    { value: "zh-CN", label: "中文" },
    { value: "en", label: "English" },
  ];

  return (
    <div className="flex flex-col h-full">
      {/* Page header */}
      <div className="mb-6">
        <h2 className="text-[1.63rem] font-bold text-zinc-900">
          {t("settings.title")}
        </h2>
      </div>

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
    </div>
  );
}
