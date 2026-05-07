import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ChatMessage } from "../../components/AIAssistant/ChatMessage";
import { getFile } from "../../api/fileApi";
import { message } from "../../components/base";
import { useChatStore } from "../../stores/chatStore";
import { useFileStore } from "../../stores/fileStore";
import type { SearchIntent, SourceChunk } from "../../types";
import styles from "./index.module.css";

export function SmartSearchResults() {
  const { t, i18n } = useTranslation();
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
      message.error(t("smartSearch.sourceMissing"));
    }
  }

  if (mode === "rag") {
    if (messages.length === 0) {
      return (
        <section className={styles.resultsEmpty}>
          <span className="material-symbols-outlined">auto_awesome</span>
          <h2>{t("smartSearch.emptyRagTitle")}</h2>
          <p>{t("smartSearch.emptyRagHint")}</p>
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
        <h2>{t("smartSearch.emptySearchTitle")}</h2>
        <p>{t("smartSearch.placeholder")}</p>
      </section>
    );
  }

  if (results.length === 0) {
    return (
      <section className={styles.resultsEmpty}>
        <span className="material-symbols-outlined">search_off</span>
        <h2>{t("smartSearch.noResults")}</h2>
        <p>{t("smartSearch.noResultsHint")}</p>
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
        <span>{t("smartSearch.resultCount", { count: results.length })}</span>
      </div>
      {results.map((source, index) => (
        <article key={`${source.id}-${index}`} className={styles.resultRow}>
          <div className={styles.fileIcon}>
            <span className="material-symbols-outlined">description</span>
          </div>
          <button className={styles.resultMain} onClick={() => void openSource(source)}>
            <strong>{source.file_name || t("smartSearch.unknownFile")}</strong>
            <span>
              {source.heading || t("smartSearch.untitledSection")} · Chunk {source.chunk_index}
            </span>
            <p>{source.snippet || source.text}</p>
          </button>
          <div className={styles.resultScore}>{Math.round(source.score * 100)}%</div>
          <button
            className={styles.openBtn}
            onClick={() => void openSource(source)}
            aria-label={t("smartSearch.openSource")}
          >
            <span className="material-symbols-outlined">open_in_new</span>
          </button>
        </article>
      ))}
    </section>
  );
}

function IntentChips({ intent }: { intent: SearchIntent }) {
  const { t, i18n } = useTranslation();

  function formatMimeChip(mime: string) {
    if (mime === "image/") return t("smartSearch.mimeImage");
    if (mime === "video/") return t("smartSearch.mimeVideo");
    if (mime === "audio/") return t("smartSearch.mimeAudio");
    if (mime.startsWith("text/")) return t("smartSearch.mimeText");
    if (mime.includes("spreadsheet")) return t("smartSearch.mimeSpreadsheet");
    if (mime.includes("presentation")) return t("smartSearch.mimePresentation");
    if (mime.includes("wordprocessing") || mime === "application/msword") return t("smartSearch.mimeWord");
    if (mime === "application/pdf") return t("smartSearch.mimePdf");
    return mime;
  }

  function buildIntentChips(intent: SearchIntent) {
    const chips: string[] = [];
    if (intent.extensions?.length) {
      chips.push(intent.extensions.map((ext) => ext.toUpperCase()).join(" / "));
    } else if (intent.mime_types?.length) {
      chips.push(intent.mime_types.map(formatMimeChip).join(" / "));
    }
    if (intent.date_from || intent.date_to) {
      const locale = i18n.language || "zh-CN";
      const dateFmt = new Intl.DateTimeFormat(locale, { year: "numeric", month: "2-digit", day: "2-digit" });
      const from = intent.date_from ? dateFmt.format(new Date(intent.date_from)) : "";
      const to = intent.date_to ? dateFmt.format(new Date(intent.date_to)) : "";
      chips.push(from && to ? `${from} ~ ${to}` : from || to);
    }
    return chips;
  }

  const chips = buildIntentChips(intent);
  if (chips.length === 0) return null;

  return (
    <div className={styles.intentChips} aria-label={t("smartSearch.parsedFilters")}>
      <span className="material-symbols-outlined">filter_alt</span>
      {chips.map((chip) => (
        <span key={chip} className={styles.intentChip}>
          {chip}
        </span>
      ))}
    </div>
  );
}
