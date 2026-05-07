import { useTranslation } from "react-i18next";
import styles from "./index.module.css";

interface HeroProps {
  onSuggestionClick: (query: string) => void;
}

export function SmartSearchHero({ onSuggestionClick }: HeroProps) {
  const { t } = useTranslation();

  const SUGGESTIONS = [
    t("hero.suggestion1"),
    t("hero.suggestion2"),
    t("hero.suggestion3"),
  ];

  return (
    <section className={styles.hero}>
      <div className={styles.heroCopy}>
        <p>Smart Search</p>
        <h1>{t("hero.title")}</h1>
        <span>{t("hero.subtitle")}</span>
      </div>

      <div className={styles.suggestions}>
        <span>{t("hero.tryLabel")}</span>
        {SUGGESTIONS.map((item) => (
          <button key={item} type="button" onClick={() => onSuggestionClick(item)}>
            {item}
          </button>
        ))}
      </div>
    </section>
  );
}
