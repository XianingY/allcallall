import type { ReactNode } from "react";
import { useSyncExternalStore } from "react";

import i18next from "i18next";
import { AlertCircle, Inbox, LoaderCircle } from "lucide-react";

// PageState deliberately reads i18next directly instead of calling
// react-i18next's useTranslation: this repo hoists react-i18next to the
// workspace root where it binds to root react (18.2.0) while web/ runs its
// own react (18.3.1). The hook then calls useContext on the wrong React copy
// (dispatcher is null) and every page test that renders these components
// crashes. The i18next core has no react dependency, so a tiny
// useSyncExternalStore subscription keeps the labels reactive on language
// change without pulling react-i18next into this shared component.
function subscribeLanguage(onChange: () => void) {
  i18next.on("languageChanged", onChange);
  return () => {
    i18next.off("languageChanged", onChange);
  };
}

function translate(key: string) {
  return i18next.isInitialized ? i18next.t(key) : key;
}

function useTranslate() {
  useSyncExternalStore(subscribeLanguage, () => i18next.language, () => i18next.language);
  return translate;
}

export function PageLoading({ label }: { label?: string }) {
  const t = useTranslate();
  return <div className="page-state"><LoaderCircle className="animate-spin" size={20} />{label ?? t("common.loading")}</div>;
}

/**
 * Empty state. Without this, a list with no rows renders as a bare container
 * and "no data yet" is indistinguishable from "the request failed" or "the
 * page is broken" - which is exactly what a new user sees on their first
 * login, before they have any conversations, meetings or contacts.
 */
export function PageEmpty({ label, hint, action }: { label: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="page-state">
      <Inbox size={20} />
      <span>{label}</span>
      {hint ? <span className="page-state-hint">{hint}</span> : null}
      {action}
    </div>
  );
}

export function PageError({ error, retry }: { error: unknown; retry?: () => void }) {
  const t = useTranslate();
  return <div className="page-state text-danger"><AlertCircle size={20} /><span>{error instanceof Error ? error.message : t("common.loadFailed")}</span>{retry && <button className="button-secondary" onClick={retry}>{t("common.retry")}</button>}</div>;
}
