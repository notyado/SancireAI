import React, { lazy, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { AppProviders } from "@/components/providers";
import { AppShell } from "@/components/app-shell";
const HomePage = lazy(() => import("@/pages/HomePage"));
const ChatPage = lazy(() => import("@/pages/ChatPage"));
const AdminPage = lazy(() => import("@/pages/AdminPage"));
const AuditPage = lazy(() => import("@/pages/AuditPage"));
const ModulesPage = lazy(() => import("@/pages/ModulesPage"));
const SettingsPage = lazy(() => import("@/pages/SettingsPage"));
const TemplatesPage = lazy(() => import("@/pages/TemplatesPage"));
const NotFound = lazy(() => import("@/pages/NotFound"));
import "@fontsource-variable/onest";
import "@/styles.css";

function App() {
  return (
    <BrowserRouter>
      <AppProviders>
        <AppShell>
          <Suspense fallback={<main className="grid min-h-screen place-items-center text-[var(--text-secondary)]">Загрузка…</main>}>
            <Routes>
              <Route path="/" element={<HomePage />} />
              <Route path="/chat/:id" element={<ChatPage />} />
              <Route path="/admin" element={<AdminPage />} />
              <Route path="/audit" element={<AuditPage />} />
              <Route path="/modules" element={<ModulesPage />} />
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/templates" element={<TemplatesPage />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
          </Suspense>
        </AppShell>
      </AppProviders>
    </BrowserRouter>
  );
}

createRoot(document.getElementById("root")!).render(<React.StrictMode><App /></React.StrictMode>);
