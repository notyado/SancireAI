"use client";
import { useState } from "react";
import { useApp } from "@/components/providers";
import { IconCheck, IconDownload, IconShield } from "@/components/icons";
type ModuleInfo = {
  id: string;
  code: string;
  title: string;
  category: string;
  description: string;
  fields: {
    key: string;
    label: string;
    placeholder: string;
    type: "text" | "date" | "choice" | "textarea";
    required?: boolean;
    options?: string[];
    helperNote?: string;
  }[];
};
const MODULES: ModuleInfo[] = [
  {
    id: "M-01",
    code: "M-01",
    title: "Приказ о вводе в эксплуатацию информационной системы",
    category: "Информационные технологии",
    description: "Ввод разработанной или модернизированной информационной системы в промышленную эксплуатацию Банка России.",
    fields: [
      {
        key: "is_name",
        label: "Наименование информационной системы",
        placeholder: "Автоматизированная система мониторинга платежей (АС МП)",
        type: "text",
        required: true,
        helperNote: "Вводится 1 раз и автоматически дублируется во всех 7 связанных пунктах приказа.",
      },
      {
        key: "basis_doc",
        label: "Документ-основание (решение/акт)",
        placeholder: "акта готовности к вводу в эксплуатацию от 12.10.2026 № 45-ИТ",
        type: "text",
        required: true,
      },
      {
        key: "modification_type",
        label: "Характер разработки",
        placeholder: "разработанного",
        type: "choice",
        options: ["разработанного", "измененного"],
        required: true,
        helperNote: "Невыбранный вариант будет удален из итогового текста (п. 3.3 Рекомендаций).",
      },
      {
        key: "tz_requisites",
        label: "Основание разработки (ТЗ/договор)",
        placeholder: "Техническим заданием от 15.01.2026 № ТЗ-12",
        type: "text",
        required: true,
      },
      {
        key: "start_date",
        label: "Дата ввода в эксплуатацию",
        placeholder: "01.11.2026",
        type: "date",
        required: true,
      },
      {
        key: "customer_dept",
        label: "Подразделение-заказчик",
        placeholder: "Департаменту финансового мониторинга",
        type: "text",
        required: true,
      },
      {
        key: "purpose",
        label: "Целевое назначение системы",
        placeholder: "автоматизация выявления подозрительных операций и формирование аналитической отчетности",
        type: "textarea",
        required: true,
      },
      {
        key: "controller_person",
        label: "Ответственный за контроль исполнения",
        placeholder: "директора Департамента информационных технологий Иванова И.И.",
        type: "text",
        required: true,
      },
      {
        key: "signatory",
        label: "Подписант (должность и ФИО)",
        placeholder: "Заместитель Председателя Банка России Петров П.П.",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-02",
    code: "M-02",
    title: "Приказ о назначении ответственных за делопроизводство",
    category: "Делопроизводство",
    description: "Назначение основного и замещающего работников за ведение делопроизводства и СЭД в подразделении.",
    fields: [
      {
        key: "department",
        label: "Структурное подразделение",
        placeholder: "Департаменте бухгалтерского учета",
        type: "text",
        required: true,
      },
      {
        key: "main_responsible",
        label: "Основной ответственный (должность, ФИО)",
        placeholder: "ведущего специалиста Смирнову Е.А.",
        type: "text",
        required: true,
      },
      {
        key: "substitute_responsible",
        label: "Замещающий на период отсутствия",
        placeholder: "главного эксперта Ковалева В.С.",
        type: "text",
        required: true,
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Руководитель Службы текущего контроля Сидоров С.С.",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-03",
    code: "M-03",
    title: "Приказ о приеме и передаче дел и имущества",
    category: "Кадры и имущество",
    description: "Оформление передачи дел, документов, печатей и прав доступа к ИС при смене руководителя подразделения.",
    fields: [
      {
        key: "reason",
        label: "Основание издания приказа",
        placeholder: "увольнением начальника Управления отчетности Васильева А.Н.",
        type: "text",
        required: true,
      },
      {
        key: "handing_person",
        label: "Сдающий дела (должность, ФИО)",
        placeholder: "начальнику Управления Васильеву А.Н.",
        type: "text",
        required: true,
      },
      {
        key: "receiving_person",
        label: "Принимающий дела (должность, ФИО)",
        placeholder: "заместителю начальника Управления Орлову Д.В.",
        type: "text",
        required: true,
      },
      {
        key: "deadline",
        label: "Срок завершения приема-передачи",
        placeholder: "28.10.2026",
        type: "date",
        required: true,
      },
      {
        key: "has_is_access",
        label: "Передаются ли права доступа к ИС?",
        placeholder: "да",
        type: "choice",
        options: ["Да (обязательна виза Департамента ИБ)", "Нет"],
        required: true,
        helperNote: "При выборе «Да» по Строке 2 Матрицы в маршрут согласования включается Департамент ИБ.",
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Председатель Организации",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-04",
    code: "M-04",
    title: "Приказ об утверждении локального нормативного акта",
    category: "Нормотворчество",
    description: "Введение в действие новых регламентов, порядков и положений Банка России.",
    fields: [
      {
        key: "goal",
        label: "Цель принятия акта",
        placeholder: "совершенствования процедур операционного контроля",
        type: "text",
        required: true,
      },
      {
        key: "act_name",
        label: "Наименование утверждаемого документа",
        placeholder: "Порядок проведения проверок операционной надежности",
        type: "text",
        required: true,
      },
      {
        key: "effective_date",
        label: "Дата вступления в силу",
        placeholder: "01.11.2026",
        type: "date",
        required: true,
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Заместитель Председателя Организации",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-05",
    code: "M-05",
    title: "Распоряжение о проведении внутреннего аудита процессов",
    category: "Внутренний контроль",
    description: "Плановый или целевой аудит бизнес-процессов подразделения независимой группой аудиторов.",
    fields: [
      {
        key: "process_name",
        label: "Наименование проверяемого процесса",
        placeholder: "Процесс управления рисками ликвидности",
        type: "text",
        required: true,
      },
      {
        key: "audit_period",
        label: "Период проведения аудита",
        placeholder: "с 05.11.2026 по 20.11.2026",
        type: "text",
        required: true,
      },
      {
        key: "audit_lead",
        label: "Руководитель аудиторской группы",
        placeholder: "главный аудитор Кузнецов М.И.",
        type: "text",
        required: true,
      },
      {
        key: "auditors_team",
        label: "Состав аудиторов",
        placeholder: "ведущий аудитор Федорова А.П., эксперт Новиков С.К.",
        type: "textarea",
        required: true,
        helperNote: "Аудиторы не должны являться работниками проверяемого подразделения.",
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Заместитель Председателя Организации",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-06",
    code: "M-06",
    title: "Распоряжение о создании проектного комитета",
    category: "Проектное управление",
    description: "Формирование коллегиального органа координации и контроля стратегического проекта.",
    fields: [
      {
        key: "project_name",
        label: "Наименование проекта",
        placeholder: "Модернизация ядра межбанковских расчетов",
        type: "text",
        required: true,
      },
      {
        key: "committee_chair",
        label: "Председатель Комитета",
        placeholder: "Заместитель Председателя Организации Григорьев А.А.",
        type: "text",
        required: true,
      },
      {
        key: "is_it_project",
        label: "Связан ли проект с ИТ или созданием ИС?",
        placeholder: "да",
        type: "choice",
        options: ["Да (обязательна виза Департамента ИТ)", "Нет"],
        required: true,
        helperNote: "При выборе «Да» по Строке 6 Матрицы обязательна виза Департамента ИТ.",
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Заместитель Председателя Организации",
        type: "text",
        required: true,
      },
    ],
  },
  {
    id: "M-07",
    code: "M-07",
    title: "Распоряжение о создании рабочей группы",
    category: "Организационные вопросы",
    description: "Формирование временного межфункционального коллектива для решения оперативной задачи.",
    fields: [
      {
        key: "group_goal",
        label: "Практическая цель создания группы",
        placeholder: "подготовка предложений по внедрению искусственного интеллекта в ДОУ",
        type: "textarea",
        required: true,
      },
      {
        key: "lead_name",
        label: "Руководитель рабочей группы",
        placeholder: "начальник Управления инноваций Белов К.Ю.",
        type: "text",
        required: true,
      },
      {
        key: "is_it_project",
        label: "Создается ли группа для ИТ-проекта или вопросов ИС?",
        placeholder: "да",
        type: "choice",
        options: ["Да (обязательна виза Департамента ИТ)", "Нет"],
        required: true,
        helperNote: "При выборе «Да» по Строке 6 Матрицы обязательна виза Департамента ИТ.",
      },
      {
        key: "deadline",
        label: "Срок предоставления предложений",
        placeholder: "15.12.2026",
        type: "date",
        required: true,
      },
      {
        key: "signatory",
        label: "Подписант",
        placeholder: "Заместитель Председателя Организации",
        type: "text",
        required: true,
      },
    ],
  },
];
export function ModuleBuilderView() {
  const { showToast } = useApp();
  const [selectedModule, setSelectedModule] = useState<ModuleInfo | null>(MODULES[0]);
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const [generating, setGenerating] = useState(false);
  function handleSelectModule(mod: ModuleInfo) {
    setSelectedModule(mod);
    const defaults: Record<string, string> = {};
    mod.fields.forEach((f) => {
      if (f.type === "choice" && f.options) {
        defaults[f.key] = f.options[0];
      }
    });
    setFormValues(defaults);
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
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          moduleId: selectedModule.id,
          values: formValues,
        }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        showToast(err.message || "Ошибка формирования документа");
        return;
      }
      const blob = await res.blob();
      const disposition = res.headers.get("Content-Disposition");
      let filename = `${selectedModule.code}_проект.docx`;
      if (disposition) {
        const match = disposition.match(/filename="?([^"]+)"?/);
        if (match && match[1]) filename = match[1];
      }
      const url = window.URL.createObjectURL(blob);
      const link = document.createElement("a");
      link.href = url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      window.URL.revokeObjectURL(url);
      showToast("Файл DOCX сформирован и очищен по Методическим рекомендациям");
    } catch (e) {
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
          7 типовых модулей Банка России
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
            {MODULES.map((mod) => {
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
