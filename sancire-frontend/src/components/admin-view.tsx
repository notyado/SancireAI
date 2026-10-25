"use client";
import { useEffect, useRef, useState } from "react";
import { useApp } from "@/components/providers";
import { getAuthToken } from "@/lib/chat-client";

type Regulation = {
  id: string;
  title: string;
  kind: string;
  filename: string;
  status: "draft" | "active" | "archived" | string;
  chunkCount: number;
  createdAt: string;
  updatedAt: string;
};

const REG_STATUSES: Record<string, { label: string; cls: string }> = {
  active: { label: "Активный", cls: "bg-emerald-500/15 text-emerald-400 border-emerald-500/30" },
  draft: { label: "На модерации", cls: "bg-amber-500/15 text-amber-400 border-amber-500/30" },
  archived: { label: "В архиве", cls: "bg-white/5 text-[var(--text-secondary)] border-white/10" },
};

function authHeader(): Record<string, string> {
  const token = getAuthToken();
  return token ? { Authorization: `Bearer ${token}` } : {};
}
type VersionItem = {
  id: string;
  code: string;
  title: string;
  isActive: boolean;
  checksum: string;
  introducedAt: string;
  metadata?: any;
};
export function AdminView() {
  const { user, setAuthModalOpen, showToast } = useApp();
  const [versions, setVersions] = useState<VersionItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [regs, setRegs] = useState<Regulation[]>([]);
  const [regBusy, setRegBusy] = useState<string | null>(null);
  const [upFile, setUpFile] = useState<File | null>(null);
  const [upTitle, setUpTitle] = useState("");
  const [upKind, setUpKind] = useState("Другое");
  const [upBusy, setUpBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const isAdmin = user?.role === "DOU_Admin";
  useEffect(() => {
    if (!isAdmin) return;
    loadVersions();
    void loadRegs();
  }, [isAdmin]);

  async function loadRegs() {
    try {
      const res = await fetch("/api/admin/regulations", { headers: authHeader() });
      if (res.ok) {
        const data = await res.json();
        setRegs(data.regulations || []);
      }
    } catch {
      showToast("Не удалось загрузить список регламентов");
    }
  }
  async function uploadRegulation(event: React.FormEvent) {
    event.preventDefault();
    if (!upFile) {
      showToast("Выберите файл регламента");
      return;
    }
    setUpBusy(true);
    try {
      const form = new FormData();
      form.append("file", upFile);
      form.append("title", upTitle.trim());
      form.append("kind", upKind);
      const res = await fetch("/api/admin/regulations/upload", { method: "POST", headers: authHeader(), body: form });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        showToast(data.message || "Не удалось загрузить файл");
        return;
      }
      showToast(data.message || "Регламент загружен");
      setUpFile(null);
      setUpTitle("");
      if (fileInput.current) fileInput.current.value = "";
      await loadRegs();
    } catch {
      showToast("Ошибка сети при загрузке регламента");
    } finally {
      setUpBusy(false);
    }
  }
  async function regAction(id: string, action: "activate" | "archive") {
    setRegBusy(id + action);
    try {
      const res = await fetch(`/api/admin/regulations/${id}/${action}`, { method: "POST", headers: authHeader() });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        showToast(data.message || "Не удалось изменить статус");
        return;
      }
      showToast(action === "activate" ? "Регламент активирован — уже используется в ответах" : "Регламент перемещён в архив");
      await loadRegs();
    } catch {
      showToast("Ошибка сети");
    } finally {
      setRegBusy(null);
    }
  }
  async function removeReg(id: string) {
    if (!window.confirm("Удалить регламент и все его фрагменты из индекса?")) return;
    setRegBusy(id + "del");
    try {
      const res = await fetch(`/api/admin/regulations/${id}`, { method: "DELETE", headers: authHeader() });
      if (!res.ok) {
        showToast("Не удалось удалить регламент");
        return;
      }
      showToast("Регламент удалён");
      await loadRegs();
    } finally {
      setRegBusy(null);
    }
  }
  async function downloadReg(id: string, filename: string) {
    try {
      const res = await fetch(`/api/admin/regulations/${id}/file`, { headers: authHeader() });
      if (!res.ok) {
        showToast("Файл не найден");
        return;
      }
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      showToast("Не удалось скачать файл");
    }
  }
  async function loadVersions() {
    setLoading(true);
    const token = getAuthToken();
    try {
      const res = await fetch("/api/admin/versions", {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setVersions(data);
      }
    } catch (e) {
      showToast("Ошибка загрузки версий");
    } finally {
      setLoading(false);
    }
  }
  async function handleActivate(code: string) {
    setActionLoading(code);
    const token = getAuthToken();
    try {
      const res = await fetch("/api/admin/versions/activate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${token}`,
        },
        body: JSON.stringify({ code }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        showToast(err.message || "Ошибка активации версии");
        return;
      }
      showToast(`Версия ${code} успешно активирована без перезагрузки системы`);
      await loadVersions();
    } catch (e) {
      showToast("Ошибка сети при активации версии");
    } finally {
      setActionLoading(null);
    }
  }
  if (!isAdmin) {
    return (
      <main className="mx-auto min-h-screen w-full max-w-[720px] px-6 pb-20 pt-28 text-center flex flex-col items-center justify-center">
        <div className="w-16 h-16 rounded-3xl bg-amber-500/10 border border-amber-500/20 text-amber-400 flex items-center justify-center text-2xl font-mono mb-4">
          403
        </div>
        <h1 className="text-[28px] font-medium tracking-tight">Доступ ограничен</h1>
        <p className="mt-2 text-[15px] text-[var(--text-secondary)] max-w-md">
          Панель управления версиями доступна только специалистам и администраторам Департамента ДОУ Банка России.
        </p>
        <div className="mt-6 flex gap-3">
          <button
            type="button"
            className="primary-btn solid px-5 py-2.5 text-sm"
            onClick={() => setAuthModalOpen(true)}
          >
            Войти как Администратор ДОУ
          </button>
        </div>
      </main>
    );
  }
  return (
    <main className="page-fade mx-auto min-h-screen w-full max-w-[960px] px-6 pb-20 pt-28 md:px-10">
      <div className="flex flex-col gap-2">
        <span className="text-[12.5px] uppercase tracking-[0.08em] text-[var(--text-secondary)]">
          Администрирование • RBAC DOU_Admin
        </span>
        <h1 className="text-[34px] font-medium tracking-tight md:text-[40px]">
          Панель управления базой знаний ДОУ
        </h1>
        <p className="max-w-2xl text-[15px] text-[var(--text-secondary)]">
          Версионирование нормативных актов (FR-028..FR-030). Переключение редакции вступает в силу
          мгновенно для всех запросов чата-консультанта без перезагрузки бэкенда.
        </p>
      </div>
      {/* System Status Cards */}
      <div className="mt-8 grid grid-cols-1 md:grid-cols-3 gap-4">
        <div className="p-5 rounded-2xl border border-[var(--divider)] bg-[var(--surface)]">
          <span className="text-[12px] uppercase text-[var(--text-secondary)]">Векторная БД Qdrant</span>
          <div className="mt-2 text-[20px] font-medium text-emerald-400">362 чанка</div>
          <div className="mt-1 text-[12.5px] text-[var(--text-secondary)]">Коллекция dou_knowledge (1024 dim)</div>
        </div>
        <div className="p-5 rounded-2xl border border-[var(--divider)] bg-[var(--surface)]">
          <span className="text-[12px] uppercase text-[var(--text-secondary)]">Инференс Ollama</span>
          <div className="mt-2 text-[20px] font-medium text-white">qwen3.5:4b</div>
          <div className="mt-1 text-[12.5px] text-[var(--text-secondary)]">Локальный GPU On-Premise</div>
        </div>
        <div className="p-5 rounded-2xl border border-[var(--divider)] bg-[var(--surface)]">
          <span className="text-[12px] uppercase text-[var(--text-secondary)]">Модель эмбеддингов</span>
          <div className="mt-2 text-[20px] font-medium text-white">qwen3-embedding</div>
          <div className="mt-1 text-[12.5px] text-[var(--text-secondary)]">Cosine Distance • Safe Refusal &gt; 0.65</div>
        </div>
      </div>
      {/* Regulation lifecycle */}
      <section className="mt-10 space-y-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <h2 className="text-[20px] font-medium">Регламенты</h2>
            <p className="mt-1 text-[13.5px] text-[var(--text-secondary)]">
              Загрузка документов → нарезка на фрагменты → выбор активной редакции (только одна на вид) → архив.
              Активный регламент сразу участвует в ответах чата и в векторном поиске Qdrant.
            </p>
          </div>
          <button type="button" className="pill text-xs" onClick={() => void loadRegs()}>
            Обновить
          </button>
        </div>

        {/* Upload card */}
        <form onSubmit={uploadRegulation} className="rounded-3xl border border-[var(--divider)] bg-[var(--surface)] p-5">
          <div className="flex flex-col gap-3 md:flex-row md:items-end">
            <label className="flex-1">
              <span className="mb-1.5 block text-[12px] uppercase tracking-[0.07em] text-[var(--text-secondary)]">Файл (PDF, DOCX, TXT, MD)</span>
              <input
                ref={fileInput}
                type="file"
                accept=".pdf,.docx,.txt,.md"
                className="w-full cursor-pointer rounded-xl border border-[var(--divider)] bg-transparent px-3 py-2.5 text-[14px] file:mr-3 file:rounded-lg file:border-0 file:bg-[var(--divider)] file:px-3 file:py-1 file:text-[13px] file:text-[var(--text)]"
                onChange={(e) => {
                  const f = e.target.files?.[0] || null;
                  setUpFile(f);
                  if (f && !upTitle) setUpTitle(f.name.replace(/\.[^.]+$/, ""));
                }}
              />
            </label>
            <label className="flex-1">
              <span className="mb-1.5 block text-[12px] uppercase tracking-[0.07em] text-[var(--text-secondary)]">Название</span>
              <input
                value={upTitle}
                onChange={(e) => setUpTitle(e.target.value)}
                placeholder="Например: Инструкция по ДОУ (редакция 2)"
                className="w-full rounded-xl border border-[var(--divider)] bg-transparent px-3 py-2.5 text-[14px] outline-none focus:border-[var(--accent)]"
              />
            </label>
            <label>
              <span className="mb-1.5 block text-[12px] uppercase tracking-[0.07em] text-[var(--text-secondary)]">Вид</span>
              <select
                value={upKind}
                onChange={(e) => setUpKind(e.target.value)}
                className="rounded-xl border border-[var(--divider)] bg-[var(--surface-solid)] px-3 py-2.5 text-[14px] outline-none focus:border-[var(--accent)]"
              >
                <option>Инструкция</option>
                <option>Матрица визирования</option>
                <option>Методические рекомендации</option>
                <option>Другое</option>
              </select>
            </label>
            <button type="submit" className="primary-btn solid px-5 py-2.5 text-sm" disabled={upBusy}>
              {upBusy ? "Загружаю…" : "Загрузить"}
            </button>
          </div>
          {upFile ? <p className="mt-2 text-[12.5px] text-[var(--text-secondary)]">Выбран: {upFile.name} ({Math.max(1, Math.round(upFile.size / 1024))} КБ)</p> : null}
        </form>

        {/* List */}
        <div className="rounded-3xl border border-[var(--divider)] bg-[var(--surface)] overflow-hidden divide-y divide-[var(--divider)]">
          {regs.length === 0 ? (
            <div className="p-6 text-[14px] text-[var(--text-secondary)]">Регламенты ещё не загружены.</div>
          ) : null}
          {regs.map((reg) => {
            const st = REG_STATUSES[reg.status] || REG_STATUSES.draft;
            return (
              <div key={reg.id} className="p-5 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2.5">
                    <span className={`inline-block border px-2 py-0.5 text-[11.5px] font-medium rounded-md ${st.cls}`}>{st.label}</span>
                    <h3 className="truncate text-[15.5px] font-medium">{reg.title}</h3>
                    <span className="text-[12px] text-[var(--text-secondary)]">• {reg.kind}</span>
                  </div>
                  <div className="mt-1.5 flex flex-wrap gap-4 text-[12.5px] text-[var(--text-secondary)]">
                    <span>{reg.filename}</span>
                    <span>{reg.chunkCount} фрагментов</span>
                    <span>обновлён {new Date(reg.updatedAt).toLocaleDateString("ru-RU")}</span>
                  </div>
                </div>
                <div className="flex flex-wrap gap-2">
                  {reg.status !== "active" ? (
                    <button
                      type="button"
                      className="primary-btn solid px-3.5 py-1.5 text-xs"
                      disabled={regBusy === reg.id + "activate"}
                      onClick={() => void regAction(reg.id, "activate")}
                    >
                      {regBusy === reg.id + "activate" ? "Активация…" : "Сделать активным"}
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="pill px-3.5 py-1.5 text-xs"
                      disabled={regBusy === reg.id + "archive"}
                      onClick={() => void regAction(reg.id, "archive")}
                    >
                      В архив
                    </button>
                  )}
                  <button type="button" className="pill px-3.5 py-1.5 text-xs" onClick={() => void downloadReg(reg.id, reg.filename)}>
                    Скачать
                  </button>
                  <button
                    type="button"
                    className="pill px-3.5 py-1.5 text-xs text-red-400"
                    disabled={regBusy === reg.id + "del"}
                    onClick={() => void removeReg(reg.id)}
                  >
                    Удалить
                  </button>
                </div>
              </div>
            );
          })}
        </div>
      </section>

      {/* Versions List */}
      <div className="mt-10 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-[20px] font-medium">Реестр версий Инструкции по ДОУ</h2>
          <button
            type="button"
            className="pill text-xs"
            onClick={loadVersions}
            disabled={loading}
          >
            {loading ? "Обновление..." : "Обновить список"}
          </button>
        </div>
        <div className="rounded-3xl border border-[var(--divider)] bg-[var(--surface)] overflow-hidden divide-y divide-[var(--divider)]">
          {versions.map((ver) => (
            <div key={ver.id} className="p-6 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
              <div className="space-y-1.5">
                <div className="flex items-center gap-2.5">
                  <span className="font-mono text-sm px-2.5 py-0.5 rounded-lg bg-[var(--divider)] font-semibold">
                    {ver.code}
                  </span>
                  {ver.isActive ? (
                    <span className="text-[11.5px] font-medium px-2 py-0.5 rounded-md bg-emerald-500/20 text-emerald-400 border border-emerald-500/30">
                      Активная редакция
                    </span>
                  ) : (
                    <span className="text-[11.5px] text-[var(--text-secondary)]">Резервная редакция</span>
                  )}
                </div>
                <h3 className="text-[16px] font-medium">{ver.title}</h3>
                <div className="flex gap-4 text-[12.5px] text-[var(--text-secondary)] pt-0.5">
                  <span>Введена: {ver.introducedAt}</span>
                  <span>Контрольная сумма SHA-256: {ver.checksum}</span>
                </div>
              </div>
              <div>
                {ver.isActive ? (
                  <span className="text-xs text-emerald-400 font-medium px-4 py-2 rounded-xl bg-emerald-500/10 border border-emerald-500/20 inline-block">
                    Используется в RAG
                  </span>
                ) : (
                  <button
                    type="button"
                    className="primary-btn solid px-4 py-2 text-xs"
                    onClick={() => handleActivate(ver.code)}
                    disabled={actionLoading === ver.code}
                  >
                    {actionLoading === ver.code ? "Активация..." : "Активировать редакцию"}
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      </div>
    </main>
  );
}
