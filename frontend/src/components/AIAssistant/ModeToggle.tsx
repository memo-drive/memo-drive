import { useTranslation } from "react-i18next";
import { useChatStore, type AIMode } from "../../stores/chatStore";
import styles from "./ModeToggle.module.css";

export function ModeToggle() {
  const { t } = useTranslation();
  const { mode, setMode } = useChatStore();

  const modes: { key: AIMode; label: string; hint: string }[] = [
    { key: "rag", label: t("ai.modeRag"), hint: t("ai.modeRagHint") },
    { key: "search", label: t("ai.modeSearch"), hint: t("ai.modeSearchHint") },
  ];

  return (
    <div className={styles.toggle} role="tablist" aria-label={t("ai.modeLabel")}>
      {modes.map((item) => {
        const active = item.key === mode;
        return (
          <button
            key={item.key}
            className={`${styles.tab} ${active ? styles.active : ""}`}
            type="button"
            role="tab"
            aria-selected={active}
            onClick={() => setMode(item.key)}
            title={item.hint}
          >
            {item.label}
          </button>
        );
      })}
    </div>
  );
}
