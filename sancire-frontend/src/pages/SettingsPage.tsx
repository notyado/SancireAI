"use client";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { useApp } from "@/components/providers";
export default function SettingsPage() {
  const { theme, setTheme, language, setLanguage, refreshConversations } = useApp();
  const [confirm, setConfirm] = useState(false);
  const navigate = useNavigate();
  async function clearHistory() {
    await fetch("/api/conversations", { method: "DELETE" });
    await refreshConversations();
    setConfirm(false);
    navigate("/");
  }
  return (
    <main className="page-fade mx-auto min-h-screen w-full max-w-[720px] px-6 pb-20 pt-28 md:px-10">
      <p className="text-[13px] uppercase tracking-[0.08em] text-[var(--text-secondary)]">Настройки</p>
      <h1 className="mt-3 text-[36px] font-medium tracking-[-0.04em]">Среда работы</h1>
      <p className="mt-3 text-[15.5px] text-[var(--text-secondary)]">
        Только то, что влияет на спокойствие интерфейса. Никаких технических подробностей.
      </p>
      <section className="settings-card mt-10">
        <h2 className="text-[18px] font-medium">Тема оформления</h2>
        <p className="mt-2 text-[14.5px] text-[var(--text-secondary)]">
          Плавный переход между светлой и тёмной палитрой.
        </p>
        <div className="segment mt-5">
          {(["dark", "light", "system"] as const).map((item) => (
            <button
              key={item}
              type="button"
              className={theme === item ? "active" : ""}
              onClick={() => setTheme(item)}
            >
              {item === "dark" ? "Тёмная" : item === "light" ? "Светлая" : "Системная"}
            </button>
          ))}
        </div>
      </section>
      <section className="settings-card mt-5">
        <h2 className="text-[18px] font-medium">Язык интерфейса</h2>
        <p className="mt-2 text-[14.5px] text-[var(--text-secondary)]">
          Ответы по регламентам остаются на языке документов.
        </p>
        <div className="segment mt-5">
          <button type="button" className={language === "ru" ? "active" : ""} onClick={() => setLanguage("ru")}>
            Русский
          </button>
          <button type="button" className={language === "en" ? "active" : ""} onClick={() => setLanguage("en")}>
            English
          </button>
        </div>
      </section>
      <section className="settings-card relative mt-5">
        <h2 className="text-[18px] font-medium">История диалогов</h2>
        <p className="mt-2 text-[14.5px] text-[var(--text-secondary)]">
          Можно начать с чистого листа. Закреплённые и обычные диалоги удаляются вместе.
        </p>
        <button type="button" className="pill mt-5" onClick={() => setConfirm(true)}>
          Очистить историю
        </button>
        {confirm ? (
          <div className="confirm-pop left-6 top-[70%]">
            <p className="mb-2 text-[13px]">Удалить все диалоги?</p>
            <div className="flex gap-1">
              <button type="button" onClick={() => setConfirm(false)}>
                Отмена
              </button>
              <button type="button" onClick={() => void clearHistory()}>
                Удалить
              </button>
            </div>
          </div>
        ) : null}
      </section>
    </main>
  );
}
