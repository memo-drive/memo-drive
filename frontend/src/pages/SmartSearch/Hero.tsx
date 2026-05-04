import styles from "./index.module.css";

const SUGGESTIONS = [
  "找一下我 2024 年的发票",
  "总结上周的会议纪要",
  "包含 OCR 文字的截图",
];

interface HeroProps {
  onSuggestionClick: (query: string) => void;
}

export function SmartSearchHero({ onSuggestionClick }: HeroProps) {
  return (
    <section className={styles.hero}>
      <div className={styles.heroCopy}>
        <p>Smart Search</p>
        <h1>发现你的数据</h1>
        <span>用自然语言搜索文件、内容或元信息，也可以直接让 AI 总结答案。</span>
      </div>

      <div className={styles.suggestions}>
        <span>试试：</span>
        {SUGGESTIONS.map((item) => (
          <button key={item} type="button" onClick={() => onSuggestionClick(item)}>
            {item}
          </button>
        ))}
      </div>
    </section>
  );
}
