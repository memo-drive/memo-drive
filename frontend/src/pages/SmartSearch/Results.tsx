import { useNavigate } from "react-router-dom";
import { ChatMessage } from "../../components/AIAssistant/ChatMessage";
import { getFile } from "../../api/fileApi";
import { message } from "../../components/base";
import { useChatStore } from "../../stores/chatStore";
import { useFileStore } from "../../stores/fileStore";
import type { SearchIntent, SourceChunk } from "../../types";
import styles from "./index.module.css";

export function SmartSearchResults() {
  const mode = useChatStore((state) => state.mode);
  const messages = useChatStore((state) => state.messages);
  const results = useChatStore((state) => state.searchResults);
  const searchQuery = useChatStore((state) => state.searchQuery);
  const searchIntent = useChatStore((state) => state.searchIntent);
  const setCurrentPath = useFileStore((state) => state.setCurrentPath);
  const setSelectedFile = useFileStore((state) => state.setSelectedFile);
  const navigate = useNavigate();

  async function openSource(source: SourceChunk) {
    try {
      const file = await getFile(source.file_id);
      setCurrentPath(file.path || "/");
      setSelectedFile(file);
      navigate("/");
    } catch {
      message.error("源文件不存在或已删除");
    }
  }

  if (mode === "rag") {
    if (messages.length === 0) {
      return (
        <section className={styles.resultsEmpty}>
          <span className="material-symbols-outlined">auto_awesome</span>
          <h2>和你的文件对话</h2>
          <p>输入问题，AI 会基于文件内容综合回答。</p>
        </section>
      );
    }
    return (
      <section className={styles.resultsList}>
        {messages.map((msg) => (
          <ChatMessage key={msg.id} message={msg} />
        ))}
      </section>
    );
  }

  if (!searchQuery) {
    return (
      <section className={styles.resultsEmpty}>
        <span className="material-symbols-outlined">travel_explore</span>
        <h2>先问一句，让 AI 帮你找。</h2>
        <p>输入关键词或问题，AI 帮你找到最相关的文件片段。</p>
      </section>
    );
  }

  if (results.length === 0) {
    return (
      <section className={styles.resultsEmpty}>
        <span className="material-symbols-outlined">search_off</span>
        <h2>暂时没有命中片段</h2>
        <p>换一个关键词，或者切到文件问答模式让 AI 综合回答。</p>
        {searchIntent ? <IntentChips intent={searchIntent} /> : null}
      </section>
    );
  }

  return (
    <section className={styles.resultsList}>
      <div className={styles.resultsHeader}>
        <div>
          <p>Search results</p>
          <h2>{searchQuery}</h2>
          {searchIntent ? <IntentChips intent={searchIntent} /> : null}
        </div>
        <span>{results.length} 条结果</span>
      </div>
      {results.map((source, index) => (
        <article key={`${source.id}-${index}`} className={styles.resultRow}>
          <div className={styles.fileIcon}>
            <span className="material-symbols-outlined">description</span>
          </div>
          <button className={styles.resultMain} onClick={() => void openSource(source)}>
            <strong>{source.file_name || "未知文件"}</strong>
            <span>
              {source.heading || "未命名段落"} · Chunk {source.chunk_index}
            </span>
            <p>{source.snippet || source.text}</p>
          </button>
          <div className={styles.resultScore}>{Math.round(source.score * 100)}%</div>
          <button
            className={styles.openBtn}
            onClick={() => void openSource(source)}
            aria-label="打开来源文件"
          >
            <span className="material-symbols-outlined">open_in_new</span>
          </button>
        </article>
      ))}
    </section>
  );
}

function IntentChips({ intent }: { intent: SearchIntent }) {
  const chips = buildIntentChips(intent);
  if (chips.length === 0) return null;

  return (
    <div className={styles.intentChips} aria-label="已解析的筛选条件">
      <span className="material-symbols-outlined">filter_alt</span>
      {chips.map((chip) => (
        <span key={chip} className={styles.intentChip}>
          {chip}
        </span>
      ))}
    </div>
  );
}

function buildIntentChips(intent: SearchIntent) {
  const chips: string[] = [];
  if (intent.extensions?.length) {
    chips.push(intent.extensions.map((ext) => ext.toUpperCase()).join(" / "));
  } else if (intent.mime_types?.length) {
    chips.push(intent.mime_types.map(formatMimeChip).join(" / "));
  }
  if (intent.date_from || intent.date_to) {
    const from = intent.date_from ? new Date(intent.date_from).toLocaleDateString("zh-CN") : "";
    const to = intent.date_to ? new Date(intent.date_to).toLocaleDateString("zh-CN") : "";
    chips.push(from && to ? `${from} ~ ${to}` : from || to);
  }
  return chips;
}

function formatMimeChip(mime: string) {
  if (mime === "image/") return "图片";
  if (mime === "video/") return "视频";
  if (mime === "audio/") return "音频";
  if (mime.startsWith("text/")) return "文本";
  if (mime.includes("spreadsheet")) return "表格";
  if (mime.includes("presentation")) return "演示文稿";
  if (mime.includes("wordprocessing") || mime === "application/msword") return "Word";
  if (mime === "application/pdf") return "PDF";
  return mime;
}
