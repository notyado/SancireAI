import { useEffect, useState } from "react";
import { useApp } from "@/components/providers";
import { jsonHeaders } from "@/lib/chat-client";
import type { ModuleInfo } from "@/db/schema";
import { IconCheck, IconDownload, IconShield } from "@/components/icons";

/** Choice fields start pre-selected, so the very first «Сформировать» click
 * cannot fail on a required option the user never touched. */
function defaultValues(mod: ModuleInfo): Record<string, string> {
  const defaults: Record<string, string> = {};
  mod.fields.forEach((f) => {
    if (f.type === "choice" && f.options?.length) defaults[f.key] = f.options[0];
  });
  return defaults;
}

/** Reads a download name from Content-Disposition, both the plain
 * `filename="…"` and the RFC 5987 `filename*=UTF-8''…` form the backend sends. */
function filenameFromDisposition(disposition: string | null, fallback: string): string {
  if (!disposition) return fallback;
  const utf8 = /filename\*=UTF-8''([^;]+)/i.exec(disposition);
  if (utf8?.[1]) {
    try {
      return decodeURIComponent(utf8[1]);
    } catch {
      // A malformed escape falls back to the plain form below.
    }
  }
  const plain = /filename="?([^";]+)"?/i.exec(disposition);
  return plain?.[1]?.trim() || fallback;
}

export function ModuleBuilderView() {
  const { showToast, modules: catalog } = useApp();
  // The catalogue is served by the generator registry (GET /api/modules) and
  // loaded once in the app context: the form always shows exactly the fields
  // the backend validates and substitutes.
  const modules = catalog ?? [];
  const loading = catalog === null;
  const [selectedModule, setSelectedModule] = useState<ModuleInfo | null>(null);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [generating, setGenerating] = useState(false);

  useEffect(() => {
    if (modules.length === 0 || selectedModule) return;
    setSelectedModule(modules[0]);
    setFormValues(defaultValues(modules[0]));
  }, [modules, selectedModule]);

  function handleSelectModule(mod: ModuleInfo) {
    setSelectedModule(mod);
    setFormValues(defaultValues(mod));
  }
  function handleFieldChange(key: string, value: string) {
    setFormValues((prev) => ({ ...prev, [key]: value }));
  }
  async function handleGenerate() {
    if (!selectedModule) return;
    setGenerating(true);
    try {
      const res = await fetch("/api/documents/generate", {
        method: "POST",
        headers: jsonHeaders(),
        body: JSON.stringify({ moduleId: selectedModule.id, values: formValues }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        showToast(err.message || "Ошибка формирования документа");
        return;
      }
      const blob = await res.blob();
      const filename = filenameFromDisposition(
        res.headers.get("Content-Disposition"),
        `${selectedModule.code}_проект.docx`,
      );
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      showToast("Файл DOCX сформирован и очищен по Методическим рекомендациям");
    } catch {
      showToast("Ошибка сети при скачивании файла");
    } finally {
      setGenerating(false);
    }
  }
  return (
    <div className="mx-auto min-h-screen w-full max-w-[1180px] px-5 pb-24 pt-24 md:px-10">
      {/* Header */}
      <div className="flex flex-col gap-2">
        <span className="text-[12px] font-medium uppercase tracking-[0.1em] text-[var(--accent)]">
          Конструктор документов
        </span>
        <h1 className="text-[30px] font-medium leading-tight tracking-[-0.03em] md:text-[38px]">
          {modules.length ? `${modules.length} типовых модулей Банка России` : "Типовые модули Банка России"}
        </h1>
        <p className="max-w-2xl text-[14.5px] leading-relaxed text-[var(--text-secondary)]">
          Выберите модуль и заполните параметры проекта. Система выполнит подстановку,
          удалит неиспользованные варианты и служебные скобки и выдаст очищенный
          <span className="text-[var(--text)]"> .docx</span> — сразу к согласованию.
        </p>
      </div>

      <div className="mt-8 grid grid-cols-1 gap-6 lg:grid-cols-12 lg:gap-8">
        {/* Module picker */}
        <div className="lg:col-span-5">
          <div className="side-section-label" style={{ padding: "0 4px 10px" }}>
            Шаг 1 — выбор модуля
          </div>
          <div className="flex flex-col gap-2.5">
            {loading ? (
              <p className="rounded-2xl border border-[var(--divider)] bg-[var(--surface)] p-4 text-[13px] text-[var(--text-secondary)]">
                Загрузка каталога модулей…
              </p>
            ) : null}
            {!loading && modules.length === 0 ? (
              <p className="rounded-2xl border border-[var(--divider)] bg-[var(--surface)] p-4 text-[13px] text-[var(--text-secondary)]">
                Каталог модулей недоступен — проверьте, что бэкенд отвечает на <code>/api/modules</code>.
              </p>
            ) : null}
            {modules.map((mod) => {
              const active = selectedModule?.id === mod.id;
              return (
                <button
                  key={mod.id}
                  type="button"
                  onClick={() => handleSelectModule(mod)}
                  className={`group w-full rounded-2xl border p-4 text-left transition-colors ${
                    active
                      ? "border-[var(--accent)]/70 bg-[color-mix(in_srgb,var(--accent)_10%,transparent)]"
                      : "border-[var(--divider)] bg-[var(--surface)] hover:border-[var(--accent)]/40"
                  }`}
                >
                  <div className="flex items-center justify-between gap-3">
                    <span
                      className={`inline-block rounded-md px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.08em] ${
                        active
                          ? "bg-[var(--accent)]/20 text-[var(--accent)]"
                          : "bg-[var(--divider)] text-[var(--text-secondary)]"
                      }`}
                    >
                      {mod.code}
                    </span>
                    <span className="truncate text-[11.5px] text-[var(--text-secondary)]">
                      {mod.category}
                    </span>
                  </div>
                  <h3 className="mt-2.5 text-[15px] font-medium leading-snug tracking-[-0.01em]">
                    {mod.title}
                  </h3>
                  <p className="mt-1.5 line-clamp-2 text-[13px] leading-relaxed text-[var(--text-secondary)]">
                    {mod.description}
                  </p>
                </button>
              );
            })}
          </div>
        </div>

        {/* Parameter form */}
        <div className="lg:col-span-7">
          {selectedModule ? (
            <div className="rounded-3xl border border-[var(--divider)] bg-[var(--surface)] p-5 md:p-7 lg:sticky lg:top-24">
              <div className="border-b border-[var(--divider)] pb-4">
                <div className="flex flex-wrap items-center gap-2.5">
                  <span className="rounded-md bg-[var(--accent)]/12 px-2 py-0.5 text-[11.5px] font-semibold uppercase tracking-[0.08em] text-[var(--accent)]">
                    {selectedModule.code}
                  </span>
                  <span className="inline-flex items-center gap-1 rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-[11px] font-medium text-emerald-400">
                    <IconCheck size={11} />
                    Утверждённый модуль
                  </span>
                  <span className="ml-auto text-[11.5px] text-[var(--text-secondary)]">
                    {selectedModule.category}
                  </span>
                </div>
                <h2 className="mt-2.5 text-[18.5px] font-medium leading-snug tracking-[-0.02em]">
                  {selectedModule.title}
                </h2>
              </div>

              <div className="mt-5 space-y-4.5">
                {selectedModule.fields.map((field) => (
                  <div key={field.key} className="space-y-1.5">
                    <label className="block text-[13px] font-medium text-[var(--text)]">
                      {field.label} {field.required ? <span className="text-red-400">*</span> : null}
                    </label>
                    {field.type === "choice" && field.options ? (
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                        {field.options.map((opt) => (
                          <button
                            key={opt}
                            type="button"
                            className={`rounded-xl border px-3.5 py-2.5 text-left text-[13.5px] leading-snug transition-colors ${
                              formValues[field.key] === opt
                                ? "border-[var(--accent)]/70 bg-[var(--accent)]/12 font-medium text-[var(--text)]"
                                : "border-[var(--divider)] text-[var(--text-secondary)] hover:border-[var(--accent)]/40"
                            }`}
                            onClick={() => handleFieldChange(field.key, opt)}
                          >
                            {opt}
                          </button>
                        ))}
                      </div>
                    ) : field.type === "textarea" ? (
                      <textarea
                        rows={3}
                        value={formValues[field.key] || ""}
                        onChange={(e) => handleFieldChange(field.key, e.target.value)}
                        placeholder={field.placeholder}
                        className="w-full resize-y rounded-xl border border-[var(--divider)] bg-[color-mix(in_srgb,var(--surface-solid)_60%,transparent)] p-3 text-[14px] outline-none transition-colors placeholder:text-[color-mix(in_srgb,var(--text-secondary)_65%,transparent)] focus:border-[var(--accent)]"
                      />
                    ) : (
                      <input
                        type="text"
                        value={formValues[field.key] || ""}
                        onChange={(e) => handleFieldChange(field.key, e.target.value)}
                        placeholder={field.placeholder}
                        className="w-full rounded-xl border border-[var(--divider)] bg-[color-mix(in_srgb,var(--surface-solid)_60%,transparent)] px-3.5 py-2.5 text-[14px] outline-none transition-colors placeholder:text-[color-mix(in_srgb,var(--text-secondary)_65%,transparent)] focus:border-[var(--accent)]"
                      />
                    )}
                    {field.helperNote ? (
                      <p className="flex items-start gap-1.5 text-[12px] leading-relaxed text-[var(--text-secondary)]">
                        <span className="mt-0.5 inline-block h-1 w-1 shrink-0 rounded-full bg-[var(--accent)]" />
                        {field.helperNote}
                      </p>
                    ) : null}
                  </div>
                ))}
              </div>

              <div className="mt-7 flex flex-col gap-3 border-t border-[var(--divider)] pt-5 sm:flex-row sm:items-center sm:justify-between">
                <div className="flex items-start gap-2 text-[12.5px] leading-relaxed text-[var(--text-secondary)]">
                  <span className="mt-0.5 text-[var(--accent)]"><IconShield size={14} /></span>
                  <span>Очистка текста по Методическим рекомендациям (п. 3.3) — без служебных скобок и сносок</span>
                </div>
                <button
                  type="button"
                  onClick={handleGenerate}
                  disabled={generating}
                  className="primary-btn solid shrink-0 px-6 py-3 text-sm"
                >
                  <IconDownload size={16} />
                  {generating ? "Формирование DOCX…" : "Сформировать .docx"}
                </button>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
}
