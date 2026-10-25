"use client";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import type { ConversationDraft, MessageMeta } from "@/db/schema";
import { getAuthToken, setAuthToken, clearAuthToken } from "@/lib/chat-client";
import { AuthModal } from "@/components/auth-modal";
export type ConversationRow = {
  id: string;
  title: string;
  pinned: boolean;
  draft: ConversationDraft;
  createdAt: string;
  updatedAt: string;
};
export type MessageRow = {
  id: string;
  conversationId: string;
  role: "user" | "assistant" | string;
  content: string;
  meta: MessageMeta | null;
  createdAt: string;
};
export type ReaderState = {
  sectionId: string;
  regulationId?: string;
} | null;
export type UserProfile = {
  id: string;
  username: string;
  role: string;
  name: string;
};
type ThemeMode = "dark" | "light" | "system";
type AppContextValue = {
  theme: ThemeMode;
  setTheme: (theme: ThemeMode) => void;
  language: string;
  setLanguage: (language: string) => void;
  conversations: ConversationRow[];
  refreshConversations: () => Promise<void>;
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;
  reader: ReaderState;
  setReader: (reader: ReaderState) => void;
  toast: string | null;
  showToast: (text: string) => void;
  headerTitle: string;
  setHeaderTitle: (title: string) => void;
  user: UserProfile | null;
  authModalOpen: boolean;
  setAuthModalOpen: (open: boolean) => void;
  login: (u: string, p: string) => Promise<void>;
  logout: () => void;
};
const AppContext = createContext<AppContextValue | null>(null);
function applyTheme(theme: ThemeMode) {
  const dark =
    theme === "dark" ||
    (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
  document.documentElement.classList.toggle("light", !dark);
}
export function AppProviders({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<ThemeMode>("dark");
  const [language, setLanguageState] = useState("ru");
  const [conversations, setConversations] = useState<ConversationRow[]>([]);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [reader, setReader] = useState<ReaderState>(null);
  const [toast, setToast] = useState<string | null>(null);
  const [headerTitle, setHeaderTitle] = useState("");
  const [user, setUser] = useState<UserProfile | null>(null);
  const [authModalOpen, setAuthModalOpen] = useState(false);
  const refreshConversations = useCallback(async () => {
    const token = getAuthToken();
    const headers: Record<string, string> = {};
    if (token) headers["Authorization"] = `Bearer ${token}`;
    const response = await fetch("/api/conversations", { headers });
    if (!response.ok) return;
    const data = (await response.json()) as ConversationRow[];
    setConversations(data);
  }, []);
  // Restore authentication and settings on mount
  useEffect(() => {
    const stored = (localStorage.getItem("sancire-theme") as ThemeMode | null) ?? "dark";
    setThemeState(stored);
    applyTheme(stored);
    // Check stored user / token
    const token = getAuthToken();
    const cachedUser = localStorage.getItem("sancire_user");
    if (cachedUser) {
      try {
        setUser(JSON.parse(cachedUser));
      } catch (e) {}
    } else if (token) {
      void fetch("/api/auth/me", {
        headers: { Authorization: `Bearer ${token}` },
      })
        .then((res) => (res.ok ? res.json() : null))
        .then((data) => {
          if (data?.user) {
            setUser(data.user);
            localStorage.setItem("sancire_user", JSON.stringify(data.user));
          } else {
            clearAuthToken();
          }
        });
    } else {
      // Default initial login as employee for smooth preview
      setUser({
        id: "usr-emp-1",
        username: "employee",
        role: "Employee",
        name: "Сотрудник (Исполнитель)",
      });
    }
    void refreshConversations();
    void fetch("/api/settings")
      .then((res) => res.json())
      .then((data: { language?: string; theme?: ThemeMode }) => {
        if (data.language) setLanguageState(data.language);
      })
      .catch(() => undefined);
  }, [refreshConversations]);
  const login = useCallback(
    async (username: string, password: string) => {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username, password }),
      });
      if (!res.ok) {
        const err = await res.json().catch(() => ({}));
        throw new Error(err.message || "Неверный логин или пароль");
      }
      const data = await res.json();
      setAuthToken(data.token);
      setUser(data.user);
      localStorage.setItem("sancire_user", JSON.stringify(data.user));
      setToast(`Добро пожаловать, ${data.user.name}`);
      window.setTimeout(() => setToast(null), 2500);
      void refreshConversations();
    },
    [refreshConversations],
  );
  const logout = useCallback(() => {
    clearAuthToken();
    setUser(null);
    setToast("Вы вышли из системы");
    window.setTimeout(() => setToast(null), 2000);
  }, []);
  const setTheme = useCallback((next: ThemeMode) => {
    setThemeState(next);
    localStorage.setItem("sancire-theme", next);
    applyTheme(next);
    void fetch("/api/settings", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ theme: next }),
    });
  }, []);
  const setLanguage = useCallback((next: string) => {
    setLanguageState(next);
    void fetch("/api/settings", {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ language: next }),
    });
  }, []);
  const showToast = useCallback((text: string) => {
    setToast(text);
    window.setTimeout(() => setToast(null), 2800);
  }, []);
  const value = useMemo(
    () => ({
      theme,
      setTheme,
      language,
      setLanguage,
      conversations,
      refreshConversations,
      sidebarOpen,
      setSidebarOpen,
      reader,
      setReader,
      toast,
      showToast,
      headerTitle,
      setHeaderTitle,
      user,
      authModalOpen,
      setAuthModalOpen,
      login,
      logout,
    }),
    [
      theme,
      setTheme,
      language,
      setLanguage,
      conversations,
      refreshConversations,
      sidebarOpen,
      reader,
      toast,
      showToast,
      headerTitle,
      user,
      authModalOpen,
      login,
      logout,
    ],
  );
  return (
    <AppContext.Provider value={value}>
      {children}
      <AuthModal />
      {toast ? <div className="toast">{toast}</div> : null}
    </AppContext.Provider>
  );
}
export function useApp() {
  const ctx = useContext(AppContext);
  if (!ctx) throw new Error("useApp must be used within AppProviders");
  return ctx;
}
