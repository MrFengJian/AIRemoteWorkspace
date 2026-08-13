import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import LanguageDetector from "i18next-browser-languagedetector";

import { en } from "@/locales/en";
import { zh } from "@/locales/zh";

// Initialise i18next. Language is auto-detected from (in order):
//   1. localStorage "lang" (user's explicit choice)
//   2. navigator.language (system / browser language)
//   3. fallback "en"
// The desktop WebView2 reports the OS language as navigator.language, so a
// Chinese Windows pre-selects zh automatically.
i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
      en: { translation: en },
      zh: { translation: zh },
    },
    fallbackLng: "en",
    supportedLngs: ["en", "zh"],
    interpolation: { escapeValue: false },
    detection: {
      order: ["localStorage", "navigator"],
      lookupLocalStorage: "lang",
      caches: ["localStorage"],
    },
  });

export default i18n;
