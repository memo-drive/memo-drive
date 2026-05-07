import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import zhCN from "./resources/zh-CN.json";
import en from "./resources/en.json";

i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      "zh-CN": { translation: zhCN },
      en: { translation: en },
    },
    fallbackLng: "zh-CN",
    lng: "zh-CN",
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      caches: ["localStorage"],
      lookupLocalStorage: "memodrive.lang",
    },
  });

export default i18n;
