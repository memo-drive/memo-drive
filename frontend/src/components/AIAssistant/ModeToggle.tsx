import { useChatStore, type AIMode } from "../../stores/chatStore";
import styles from "./ModeToggle.module.css";

const modes: { key: AIMode; label: string; hint: string }[] = [
  { key: "rag", label: "文件问答", hint: "基于已索引文件回答" },
  { key: "search", label: "语义搜索", hint: "查找相关来源片段" },
];

export function ModeToggle() {
  const { mode, setMode } = useChatStore();

  return (
    <div className={styles.toggle} role="tablist" aria-label="AI 模式">
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
