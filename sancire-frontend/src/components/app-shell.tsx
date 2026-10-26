import type { ReactNode } from "react";
import { useNavigate } from "react-router-dom";
import { IconMenu, IconMoon, IconSun } from "@/components/icons";
import { ReaderPanel } from "@/components/reader-panel";
import { Sidebar } from "@/components/sidebar";
import { useApp } from "@/components/providers";
export function AppShell({ children }: { children: ReactNode }) {
  const { sidebarOpen, setSidebarOpen, theme, setTheme, headerTitle } = useApp();
  const navigate = useNavigate();
  const dark = theme !== "light";
  return (
    <>
      <div className="aurora" aria-hidden>
        <div className="aurora-blob a" />
        <div className="aurora-blob b" />
        <div className="aurora-blob c" />
      </div>
      <div className="grain" aria-hidden />
      <Sidebar />
      <ReaderPanel />
      <div className={`app-stage ${sidebarOpen ? "dimmed" : ""}`}>
        <header className="app-topbar pointer-events-none absolute inset-x-0 top-0 z-20 flex items-center justify-between px-5 py-5 md:px-8">
          <button
            type="button"
            className="icon-btn pointer-events-auto"
            onClick={() => setSidebarOpen(true)}
            aria-label="История диалогов"
          >
            <IconMenu />
          </button>
          {headerTitle ? <div className="app-topbar-title">{headerTitle}</div> : null}
          <div className="pointer-events-auto flex items-center gap-1">
            <button
              type="button"
              className="top-templates-btn pointer-events-auto rounded-[16px] px-3 py-2 text-[13px] text-[var(--text-secondary)] transition hover:bg-[var(--divider)] hover:text-[var(--text)]"
              onClick={() => navigate("/templates")}
            >
              Шаблоны
            </button>
            <button
              type="button"
              className="icon-btn"
              onClick={() => setTheme(dark ? "light" : "dark")}
              aria-label="Переключить тему"
            >
              {dark ? <IconSun /> : <IconMoon />}
            </button>
          </div>
        </header>
        {children}
      </div>
    </>
  );
}
