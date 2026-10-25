"use client";
import { memo, useMemo, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { conversationGroup } from "@/lib/dates";
import {
  IconChat,
  IconGrid,
  IconLibrary,
  IconLogo,
  IconPin,
  IconPlus,
  IconSearch,
  IconSettings,
  IconShield,
  IconSliders,
  IconTrash,
  IconUser,
  IconX,
} from "@/components/icons";
import { useApp, type ConversationRow } from "@/components/providers";
const GROUP_ORDER = ["Сегодня", "Вчера", "На прошлой неделе", "Ранее"];
export function Sidebar() {
  const { conversations, sidebarOpen, setSidebarOpen, refreshConversations, user, setAuthModalOpen } = useApp();
  const [query, setQuery] = useState("");
  const [histLimit, setHistLimit] = useState(60);
  const [confirmId, setConfirmId] = useState<string | null>(null);
  const [leaving, setLeaving] = useState<string | null>(null);
  const routeTo = useNavigate();
  const location = useLocation();
  const pathname = location.pathname;
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return conversations.filter((item) => !q || item.title.toLowerCase().includes(q));
  }, [conversations, query]);
  const pinned = filtered.filter((item) => item.pinned);
  const restAll = filtered.filter((item) => !item.pinned);
  const rest = restAll.slice(0, histLimit);
  const moreCount = restAll.length - rest.length;
  const grouped = GROUP_ORDER.map((label) => ({
    label,
    items: rest.filter((item) => conversationGroup(item.updatedAt) === label),
  })).filter((group) => group.items.length);
  async function togglePin(item: ConversationRow) {
    await fetch(`/api/conversations/${item.id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ pinned: !item.pinned }),
    });
    await refreshConversations();
  }
  async function remove(id: string) {
    setLeaving(id);
    window.setTimeout(async () => {
      await fetch(`/api/conversations/${id}`, { method: "DELETE" });
      await refreshConversations();
      setConfirmId(null);
      setLeaving(null);
      if (pathname === `/chat/${id}`) routeTo("/");
    }, 260);
  }
  function openChat(id: string) {
    setSidebarOpen(false);
    routeTo(`/chat/${id}`);
  }
  function navigate(path: string) {
    setSidebarOpen(false);
    routeTo(path);
  }
  return (
    <>
      <div
        className={`sidebar-backdrop ${sidebarOpen ? "open" : ""}`}
        onClick={() => setSidebarOpen(false)}
      />
      <aside className={`sidebar-panel ${sidebarOpen ? "open" : ""}`}>
        {/* Brand Header */}
        <div className="flex items-center justify-between px-5 pt-5">
          <div className="flex items-center gap-2.5">
            <IconLogo />
            <div className="text-[16px] font-medium tracking-tight">SancireAI</div>
          </div>
          <button type="button" className="icon-btn" onClick={() => setSidebarOpen(false)} aria-label="Закрыть">
            <IconX />
          </button>
        </div>
        {/* Primary navigation */}
        <nav className="side-nav" aria-label="Разделы">
          <button
            type="button"
            className={`nav-item ${pathname === "/" || pathname.startsWith("/chat") ? "active" : ""}`}
            onClick={() => navigate("/")}
          >
            <span className="nav-ico"><IconChat size={17} /></span>
            <span>Консультант</span>
          </button>
          <button
            type="button"
            className={`nav-item ${pathname === "/modules" ? "active" : ""}`}
            onClick={() => navigate("/modules")}
          >
            <span className="nav-ico"><IconGrid size={17} /></span>
            <span>Конструктор документов</span>
            <span className="nav-badge">7</span>
          </button>
          <button
            type="button"
            className={`nav-item ${pathname === "/audit" ? "active" : ""}`}
            onClick={() => navigate("/audit")}
          >
            <span className="nav-ico"><IconShield size={17} /></span>
            <span>Аудитор DOCX</span>
          </button>
          {user?.role === "DOU_Admin" ? (
            <button
              type="button"
              className={`nav-item ${pathname === "/admin" ? "active" : ""}`}
              onClick={() => navigate("/admin")}
            >
              <span className="nav-ico"><IconSliders size={17} /></span>
              <span>Панель ДОУ</span>
            </button>
          ) : null}
        </nav>
        <div className="side-divider" />
        <div className="px-4 pt-3">
          <label className="search-field">
            <IconSearch size={16} />
            <input
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setHistLimit(60);
              }}
              placeholder="Поиск по истории"
            />
          </label>
          <button
            type="button"
            className="primary-btn solid mt-3 w-full"
            onClick={() => {
              setSidebarOpen(false);
              navigate("/");
            }}
          >
            <IconPlus size={16} />
            Новый диалог
          </button>
        </div>
        {/* Conversation List */}
        <div className="mt-4 flex-1 overflow-y-auto px-3 pb-4">
          {pinned.length ? (
            <section className="mb-4">
              <p className="side-section-label" style={{ padding: "4px 12px 6px" }}>
                Закреплённые
              </p>
              {pinned.map((item) => (
                <HistoryRow
                  key={item.id}
                  item={item}
                  active={pathname === `/chat/${item.id}`}
                  leaving={leaving === item.id}
                  confirm={confirmId === item.id}
                  onOpen={() => openChat(item.id)}
                  onPin={() => void togglePin(item)}
                  onAskDelete={() => setConfirmId(item.id)}
                  onCancel={() => setConfirmId(null)}
                  onConfirm={() => void remove(item.id)}
                />
              ))}
            </section>
          ) : null}
          {grouped.map((group) => (
            <section key={group.label} className="mb-4">
              <p className="side-section-label" style={{ padding: "4px 12px 6px" }}>
                {group.label}
              </p>
              {group.items.map((item) => (
                <HistoryRow
                  key={item.id}
                  item={item}
                  active={pathname === `/chat/${item.id}`}
                  leaving={leaving === item.id}
                  confirm={confirmId === item.id}
                  onOpen={() => openChat(item.id)}
                  onPin={() => void togglePin(item)}
                  onAskDelete={() => setConfirmId(item.id)}
                  onCancel={() => setConfirmId(null)}
                  onConfirm={() => void remove(item.id)}
                />
              ))}
            </section>
          ))}
          {moreCount > 0 ? (
            <button
              type="button"
              className="side-more"
              onClick={() => setHistLimit((value) => value + 80)}
            >
              Показать ещё {moreCount}
            </button>
          ) : null}
        </div>
        {/* User card & footer */}
        <div className="border-t border-[var(--divider)] p-3 space-y-2">
          {user ? (
            <button
              type="button"
              onClick={() => setAuthModalOpen(true)}
              className="w-full flex items-center justify-between p-2.5 rounded-xl bg-[var(--divider)]/60 hover:bg-[var(--divider)] transition-all text-left"
            >
              <div className="min-w-0 flex-1 pr-2">
                <div className="text-[13px] font-medium truncate">{user.name}</div>
                <div className="flex items-center gap-1 text-[11px] text-[var(--text-secondary)]">
                  <IconUser size={12} />
                  {user.role === "DOU_Admin" ? "Администратор ДОУ" : "Сотрудник"}
                </div>
              </div>
              <span className="text-[11px] text-emerald-400 font-medium">Сменить</span>
            </button>
          ) : (
            <button
              type="button"
              onClick={() => setAuthModalOpen(true)}
              className="w-full py-2 text-center text-[13px] font-medium text-emerald-400 rounded-xl border border-emerald-500/30 hover:bg-emerald-500/10"
            >
              Войти в систему
            </button>
          )}
          <div className="flex gap-1">
            <button
              type="button"
              className="flex flex-1 items-center justify-center gap-1.5 rounded-xl px-2.5 py-1.5 text-[12.5px] text-[var(--text-secondary)] hover:bg-[var(--divider)]"
              onClick={() => navigate("/templates")}
            >
              <IconLibrary size={15} />
              Шаблоны
            </button>
            <button
              type="button"
              className="flex flex-1 items-center justify-center gap-1.5 rounded-xl px-2.5 py-1.5 text-[12.5px] text-[var(--text-secondary)] hover:bg-[var(--divider)]"
              onClick={() => navigate("/settings")}
            >
              <IconSettings size={15} />
              Настройки
            </button>
          </div>
        </div>
      </aside>
    </>
  );
}
const HistoryRow = memo(function HistoryRow({
  item,
  active,
  leaving,
  confirm,
  onOpen,
  onPin,
  onAskDelete,
  onCancel,
  onConfirm,
}: {
  item: ConversationRow;
  active: boolean;
  leaving: boolean;
  confirm: boolean;
  onOpen: () => void;
  onPin: () => void;
  onAskDelete: () => void;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  return (
    <div className={`relative ${leaving ? "history-item leaving" : ""}`}>
      <div className={`history-item ${active ? "active" : ""}`}>
        <button type="button" className="min-w-0 flex-1 truncate text-left text-[14px]" onClick={onOpen}>
          {item.title}
        </button>
        <span className="row-actions">
          <button
            type="button"
            className="icon-btn h-8 w-8"
            onClick={onPin}
            aria-label={item.pinned ? "Открепить" : "Закрепить"}
          >
            <IconPin size={15} />
          </button>
          <button type="button" className="icon-btn h-8 w-8" onClick={onAskDelete} aria-label="Удалить">
            <IconTrash size={15} />
          </button>
        </span>
      </div>
      {confirm ? (
        <div className="confirm-pop right-2 top-10">
          <p className="mb-2 text-[13px]">Удалить диалог?</p>
          <div className="flex gap-1">
            <button type="button" onClick={onCancel}>
              Отмена
            </button>
            <button type="button" onClick={onConfirm}>
              Удалить
            </button>
          </div>
        </div>
      ) : null}
    </div>
  );
});
