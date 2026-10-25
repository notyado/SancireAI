"use client";
import { useState, type DragEvent } from "react";
import { useApp } from "@/components/providers";
import { IconPaperclip, IconX } from "@/components/icons";
type VisaRequirement = {
  department: string;
  required: boolean;
  reason: string;
  deadline: string;
};
type RouteStep = {
  order: number;
  stage: string;
  actor: string;
  description: string;
  deadline: string;
};
type AuditFinding = {
  id: string;
  severity: "CRITICAL" | "WARNING" | "INFO";
  title: string;
  description: string;
  location?: string;
  ruleCitation: string;
  recommendation: string;
};
type AuditReport = {
  fileName: string;
  fileSize: number;
  status: "READY" | "WARNINGS" | "CRITICAL";
  statusSummary: string;
  detectedKind: string;
  detectedModule?: string;
  matchedModuleID?: string;
  totalFindings: number;
  criticalCount: number;
  warningCount: number;
  infoCount: number;
  findings: AuditFinding[];
  visaRouting: {
    kind: string;
    matchedRows: any[];
    requiredVisas: VisaRequirement[];
    routeSteps: RouteStep[];
    modulePrivilege: boolean;
    privilegeWarning?: string;
    normativeBasis: string;
  };
};
export function AuditView() {
  const { showToast } = useApp();
  const [file, setFile] = useState<File | null>(null);
  const [dragging, setDragging] = useState(false);
  const [loading, setLoading] = useState(false);
  const [report, setReport] = useState<AuditReport | null>(null);
  async function handleFileSelect(selected: File) {
    if (!selected.name.toLowerCase().endsWith(".docx")) {
      showToast("Поддерживаются только файлы формата .docx");
      return;
    }
    if (selected.size > 15 * 1024 * 1024) {
      showToast("Размер файла превышает 15 МБ");
      return;
    }
    setFile(selected);
    setLoading(true);
    const formData = new FormData();
    formData.append("file", selected);
    try {
      const res = await fetch("/api/documents/audit", {
        method: "POST",
        body: formData,
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        showToast(err.message || "Ошибка при аудите документа");
        return;
      }
      const data = (await res.json()) as AuditReport;
      setReport(data);
      showToast("Аудит документа успешно завершен");
    } catch (e) {
      showToast("Ошибка сети при отправке файла");
    } finally {
      setLoading(false);
    }
  }
  function onDragOver(e: DragEvent) {
    e.preventDefault();
    setDragging(true);
  }
  function onDragLeave() {
    setDragging(false);
  }
  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragging(false);
    if (e.dataTransfer.files && e.dataTransfer.files[0]) {
      void handleFileSelect(e.dataTransfer.files[0]);
    }
  }
  return (
    <div className="mx-auto min-h-screen w-full max-w-[1180px] px-6 pb-24 pt-24 md:px-10">
      <div className="flex flex-col gap-2">
        <span className="text-[12.5px] uppercase tracking-[0.08em] text-[var(--text-secondary)]">
          Level 3 • Интеллектуальный аудитор
        </span>
        <h1 className="text-[34px] font-medium tracking-tight md:text-[40px]">
          Аудит проекта документа и расчет Матрицы
        </h1>
        <p className="max-w-2xl text-[15px] leading-relaxed text-[var(--text-secondary)]">
          Загрузите черновик в формате DOCX. Система выявит незаполненные поля, смысловые искажения эталона
          и детерминированно рассчитает полный маршрут визирования по Матрице Банка России.
        </p>
      </div>
      {/* Upload Drop Zone */}
      <div className="mt-8">
        <div
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
          className={`relative rounded-3xl border-2 border-dashed p-10 text-center transition-all ${
            dragging
              ? "border-emerald-500 bg-emerald-500/10 scale-[1.01]"
              : "border-[var(--divider)] hover:border-white/30 bg-[var(--surface)]"
          }`}
        >
          <input
            type="file"
            accept=".docx"
            id="audit-file-input"
            className="hidden"
            onChange={(e) => {
              if (e.target.files && e.target.files[0]) {
                void handleFileSelect(e.target.files[0]);
              }
            }}
          />
          <div className="flex flex-col items-center justify-center gap-3">
            <div className="w-14 h-14 rounded-2xl bg-[var(--divider)] flex items-center justify-center text-[var(--text-secondary)]">
              <IconPaperclip size={28} />
            </div>
            <div>
              <label
                htmlFor="audit-file-input"
                className="text-[16px] font-medium hover:underline cursor-pointer text-white"
              >
                Выберите файл проекта .docx
              </label>
              <span className="text-[15px] text-[var(--text-secondary)]"> или перетащите его сюда</span>
            </div>
            <p className="text-[12.5px] text-[var(--text-secondary)]">
              Максимальный размер: 15 МБ. Поддерживаются файлы WordprocessingML (DOCX).
            </p>
          </div>
          {loading && (
            <div className="absolute inset-0 bg-[var(--surface)]/90 backdrop-blur-sm rounded-3xl flex flex-col items-center justify-center gap-3">
              <div className="w-8 h-8 rounded-full border-2 border-emerald-500 border-t-transparent animate-spin" />
              <p className="text-sm font-medium">Анализ XML-структуры и расчет Матрицы...</p>
            </div>
          )}
        </div>
      </div>
      {/* Results View */}
      {report && (
        <div className="mt-10 space-y-8 animate-enter">
          {/* Status Header Card */}
          <div
            className={`rounded-3xl border p-6 md:p-8 flex flex-col md:flex-row items-start md:items-center justify-between gap-6 ${
              report.status === "READY"
                ? "border-emerald-500/40 bg-emerald-500/10"
                : report.status === "WARNINGS"
                  ? "border-amber-500/40 bg-amber-500/10"
                  : "border-red-500/40 bg-red-500/10"
            }`}
          >
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <span
                  className={`px-3 py-1 rounded-full text-xs font-semibold uppercase tracking-wider ${
                    report.status === "READY"
                      ? "bg-emerald-500 text-black"
                      : report.status === "WARNINGS"
                        ? "bg-amber-400 text-black"
                        : "bg-red-500 text-white"
                  }`}
                >
                  {report.status === "READY"
                    ? "Готов к визированию"
                    : report.status === "WARNINGS"
                      ? "Требует внимания"
                      : "Критические ошибки"}
                </span>
                <span className="text-sm text-[var(--text-secondary)]">{report.fileName}</span>
              </div>
              <h2 className="text-[22px] font-medium tracking-tight">{report.statusSummary}</h2>
              <div className="flex flex-wrap gap-4 text-[13px] text-[var(--text-secondary)] pt-1">
                <span>Вид: <strong>{report.detectedKind}</strong></span>
                {report.detectedModule && (
                  <span>Модуль: <strong>{report.detectedModule}</strong></span>
                )}
                <span>Критических: <strong className="text-red-400">{report.criticalCount}</strong></span>
                <span>Предупреждений: <strong className="text-amber-400">{report.warningCount}</strong></span>
              </div>
            </div>
            {report.visaRouting.privilegeWarning && (
              <div className="max-w-md p-4 rounded-2xl bg-amber-500/15 border border-amber-500/30 text-[12.5px] leading-relaxed text-amber-200">
                {report.visaRouting.privilegeWarning}
              </div>
            )}
          </div>
          <div className="grid grid-cols-1 lg:grid-cols-12 gap-8">
            {/* Findings List (7 cols) */}
            <div className="lg:col-span-7 space-y-4">
              <div className="flex items-center justify-between">
                <h3 className="text-[18px] font-medium">Замечания аудитора ({report.totalFindings})</h3>
                <span className="text-[12.5px] text-[var(--text-secondary)]">Инструкция по ДОУ & Рекомендации</span>
              </div>
              {report.findings.length === 0 ? (
                <div className="rounded-2xl border border-emerald-500/30 bg-emerald-500/5 p-6 text-center text-sm text-emerald-300">
                  Замечаний не обнаружено. Все реквизиты, даты, формулировки и зоны ЭП оформлены корректно.
                </div>
              ) : (
                <div className="space-y-3">
                  {report.findings.map((f) => (
                    <div
                      key={f.id}
                      className="rounded-2xl border border-[var(--divider)] bg-[var(--surface)] p-5 space-y-3"
                    >
                      <div className="flex items-center justify-between">
                        <div className="flex items-center gap-2">
                          <span
                            className={`px-2 py-0.5 rounded text-[11px] font-semibold ${
                              f.severity === "CRITICAL"
                                ? "bg-red-500/20 text-red-400 border border-red-500/30"
                                : f.severity === "WARNING"
                                  ? "bg-amber-500/20 text-amber-300 border border-amber-500/30"
                                  : "bg-blue-500/20 text-blue-300 border border-blue-500/30"
                            }`}
                          >
                            {f.severity === "CRITICAL" ? "Критическая" : f.severity === "WARNING" ? "Предупреждение" : "Инфо"}
                          </span>
                          <span className="text-[14.5px] font-medium">{f.title}</span>
                        </div>
                        {f.location && (
                          <span className="text-[11.5px] text-[var(--text-secondary)]">{f.location}</span>
                        )}
                      </div>
                      <p className="text-[13.5px] leading-relaxed text-[var(--text-secondary)]">{f.description}</p>
                      <div className="pt-2 border-t border-[var(--divider)] flex flex-col gap-1 text-[12px]">
                        <span className="text-[var(--text-secondary)]">
                          Нормативное основание: <strong className="text-white">{f.ruleCitation}</strong>
                        </span>
                        <span className="text-emerald-400">
                          Рекомендация: {f.recommendation}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
            {/* Matrix Visa Route (5 cols) */}
            <div className="lg:col-span-5 space-y-4">
              <div>
                <h3 className="text-[18px] font-medium">Маршрут визирования по Матрице</h3>
                <p className="text-[12.5px] text-[var(--text-secondary)] mt-0.5">
                  Детерминированный расчет согласно Матрице Банка России
                </p>
              </div>
              <div className="rounded-2xl border border-[var(--divider)] bg-[var(--surface)] overflow-hidden">
                <div className="p-4 border-b border-[var(--divider)] text-[12.5px] font-medium bg-[var(--divider)]/40">
                  Обязательные визирующие подразделения:
                </div>
                <div className="divide-y divide-[var(--divider)]">
                  {report.visaRouting.requiredVisas.map((v) => (
                    <div key={v.department} className="p-4 space-y-1">
                      <div className="flex items-center justify-between">
                        <span className="font-medium text-[14px]">{v.department}</span>
                        <span className="text-[11.5px] text-emerald-400 font-mono font-medium">Обязательно</span>
                      </div>
                      <p className="text-[12px] text-[var(--text-secondary)]">{v.reason}</p>
                      <div className="text-[11.5px] text-[var(--text-secondary)] pt-0.5">
                        Срок рассмотрения: <strong>{v.deadline}</strong>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
              {/* Sequential Route Chapter 10 */}
              <div className="rounded-2xl border border-[var(--divider)] bg-[var(--surface)] p-5 space-y-3">
                <div className="font-medium text-[14px] border-b border-[var(--divider)] pb-2">
                  Последовательность этапов (Глава 10 Инструкции):
                </div>
                <div className="space-y-3 pt-1">
                  {report.visaRouting.routeSteps.map((step) => (
                    <div key={step.order} className="flex gap-3 text-[13px]">
                      <span className="w-5 h-5 rounded-full bg-[var(--divider)] flex items-center justify-center text-[11px] font-mono shrink-0">
                        {step.order}
                      </span>
                      <div>
                        <div className="font-medium">{step.stage}</div>
                        <div className="text-[12px] text-[var(--text-secondary)]">{step.actor}</div>
                        <div className="text-[11px] text-emerald-400/80">{step.deadline}</div>
                      </div>
                    </div>
                  ))}
                </div>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
