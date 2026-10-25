"use client";
import { useState } from "react";
import { useApp } from "@/components/providers";
import { IconX } from "@/components/icons";
export function AuthModal() {
  const { authModalOpen, setAuthModalOpen, login, user, logout } = useApp();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  if (!authModalOpen) return null;
  async function handleLogin(u: string, p: string) {
    setLoading(true);
    setError(null);
    try {
      await login(u, p);
      setAuthModalOpen(false);
    } catch (err: any) {
      setError(err?.message || "Ошибка авторизации");
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className="modal-layer" onClick={() => setAuthModalOpen(false)}>
      <div className="modal-card max-w-md" onClick={(e) => e.stopPropagation()}>
        <div className="flex items-center justify-between pb-4 border-b border-[var(--divider)]">
          <div>
            <h3 className="text-[18px] font-medium tracking-tight">Корпоративный вход</h3>
            <p className="text-[13px] text-[var(--text-secondary)] mt-0.5">SancireAI — Документооборот Банка России</p>
          </div>
          <button
            type="button"
            className="icon-btn"
            onClick={() => setAuthModalOpen(false)}
            aria-label="Закрыть"
          >
            <IconX size={18} />
          </button>
        </div>
        {user ? (
          <div className="mt-6 space-y-4">
            <div className="p-4 rounded-2xl bg-[var(--divider)] flex flex-col gap-1">
              <span className="text-[12px] uppercase text-[var(--text-secondary)] tracking-wider">Вы вошли как:</span>
              <span className="font-medium text-[16px]">{user.name}</span>
              <span className="text-[13px] text-[var(--text-secondary)]">
                Роль: <strong className="text-white">{user.role === "DOU_Admin" ? "Администратор ДОУ" : "Сотрудник"}</strong> (@{user.username})
              </span>
            </div>
            <button
              type="button"
              className="pill w-full py-2.5 text-center text-red-400 hover:text-red-300"
              onClick={() => {
                logout();
                setAuthModalOpen(false);
              }}
            >
              Выйти из аккаунта
            </button>
          </div>
        ) : (
          <div className="mt-6 space-y-5">
            <div className="space-y-2">
              <span className="text-[12px] uppercase tracking-wider text-[var(--text-secondary)]">Быстрый демо-вход:</span>
              <div className="grid grid-cols-2 gap-2">
                <button
                  type="button"
                  className="p-3 rounded-xl border border-[var(--divider)] text-left hover:border-emerald-500/50 hover:bg-emerald-500/5 transition-all"
                  onClick={() => handleLogin("employee", "employee123")}
                  disabled={loading}
                >
                  <div className="font-medium text-[14px]">Сотрудник</div>
                  <div className="text-[11.5px] text-[var(--text-secondary)] mt-0.5">Консультации, создание и аудит</div>
                </button>
                <button
                  type="button"
                  className="p-3 rounded-xl border border-[var(--divider)] text-left hover:border-amber-500/50 hover:bg-amber-500/5 transition-all"
                  onClick={() => handleLogin("admin", "admin123")}
                  disabled={loading}
                >
                  <div className="font-medium text-[14px]">Администратор ДОУ</div>
                  <div className="text-[11.5px] text-[var(--text-secondary)] mt-0.5">Управление версиями БЗ</div>
                </button>
              </div>
            </div>
            <div className="relative flex py-2 items-center">
              <div className="flex-grow border-t border-[var(--divider)]"></div>
              <span className="flex-shrink mx-4 text-[12px] text-[var(--text-secondary)]">или логин с паролем</span>
              <div className="flex-grow border-t border-[var(--divider)]"></div>
            </div>
            {error && (
              <div className="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-xs">
                {error}
              </div>
            )}
            <form
              onSubmit={(e) => {
                e.preventDefault();
                handleLogin(username, password);
              }}
              className="space-y-3"
            >
              <div>
                <label className="text-[12.5px] text-[var(--text-secondary)] mb-1 block">Имя пользователя</label>
                <input
                  type="text"
                  required
                  value={username}
                  onChange={(e) => setUsername(e.target.value)}
                  placeholder="employee или admin"
                  className="w-full px-3.5 py-2.5 rounded-xl border border-[var(--divider)] bg-transparent text-sm outline-none focus:border-white/40"
                />
              </div>
              <div>
                <label className="text-[12.5px] text-[var(--text-secondary)] mb-1 block">Пароль</label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="w-full px-3.5 py-2.5 rounded-xl border border-[var(--divider)] bg-transparent text-sm outline-none focus:border-white/40"
                />
              </div>
              <button
                type="submit"
                disabled={loading}
                className="primary-btn solid w-full py-2.5 mt-2 text-sm justify-center font-medium"
              >
                {loading ? "Авторизация..." : "Войти"}
              </button>
            </form>
          </div>
        )}
      </div>
    </div>
  );
}
