import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useApp } from "@/components/providers";
import { authHeader, createConversation } from "@/lib/chat-client";

/** One row of GET /api/templates (store.Template). */
type TemplateRow = {
  id: string;
  title: string;
  category: string;
  description: string;
  popular: boolean;
  size: string;
};
export default function TemplatesPage() {
  const [items, setItems] = useState<TemplateRow[]>([]);
  const [category, setCategory] = useState("Все");
  const [pending, setPending] = useState<string | null>(null);
  const navigate = useNavigate();
  const { refreshConversations, showToast } = useApp();
  useEffect(() => {
    let cancelled = false;
    void fetch("/api/templates", { headers: authHeader() })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error(String(res.status)))))
      .then((data: TemplateRow[]) => {
        if (!cancelled) setItems(data ?? []);
      })
      .catch(() => {
        if (!cancelled) showToast("Не удалось загрузить библиотеку шаблонов");
      });
    return () => {
      cancelled = true;
    };
  }, [showToast]);
  const categories = useMemo(
    () => ["Все", ...Array.from(new Set(items.map((item) => item.category)))],
    [items],
  );
  const visible = items.filter((item) => category === "Все" || item.category === category);
  const popular = visible.filter((item) => item.popular);
  const rest = visible.filter((item) => !item.popular);
  // The assistant resolves the template from the title, so the prompt is built
  // from the catalogue itself instead of a hard-coded id → phrase mapping.
  async function start(item: TemplateRow) {
    setPending(item.id);
    try {
      const conv = await createConversation(item.title);
      sessionStorage.setItem(
        `sancire_pending_prompt_${conv.id}`,
        JSON.stringify({ text: `Нужен шаблон: ${item.title}`, files: [] }),
      );
      await refreshConversations();
      navigate(`/chat/${conv.id}`);
    } catch {
      setPending(null);
      showToast("Не удалось открыть диалог — попробуйте ещё раз");
    }
  }
  return (
    <main className="page-fade mx-auto min-h-screen w-full max-w-[1080px] px-6 pb-20 pt-28 md:px-10">
      <p className="text-[13px] uppercase tracking-[0.08em] text-[var(--text-secondary)]">Библиотека</p>
      <h1 className="mt-3 text-[36px] font-medium tracking-[-0.04em] md:text-[42px]">Шаблоны документов</h1>
      <p className="mt-3 max-w-xl text-[15.5px] text-[var(--text-secondary)]">
        Выберите заготовку — помощник заполнит её в диалоге, без лишних полей и канцелярита.
      </p>
      <div className="segment mt-8">
        {categories.map((item) => (
          <button
            key={item}
            type="button"
            className={item === category ? "active" : ""}
            onClick={() => setCategory(item)}
          >
            {item}
          </button>
        ))}
      </div>
      <div className="bento mt-8">
        {popular.map((item) => (
          <article key={item.id} className="bento-card lg flex flex-col justify-between">
            <div>
              <p className="text-[12.5px] text-[var(--text-secondary)]">{item.category}</p>
              <h2 className="mt-3 text-[26px] font-medium tracking-[-0.03em]">{item.title}</h2>
              <p className="mt-3 text-[15px] leading-relaxed text-[var(--text-secondary)]">{item.description}</p>
            </div>
            <button
              type="button"
              className="primary-btn solid mt-8 self-start px-5 py-2.5 text-sm"
              onClick={() => void start(item)}
              disabled={pending === item.id}
            >
              Заполнить в диалоге
            </button>
          </article>
        ))}
        {rest.map((item) => (
          <article key={item.id} className="bento-card flex flex-col justify-between">
            <div>
              <p className="text-[12.5px] text-[var(--text-secondary)]">{item.category}</p>
              <h2 className="mt-3 text-[18px] font-medium tracking-[-0.02em]">{item.title}</h2>
              <p className="mt-2 text-[14px] leading-relaxed text-[var(--text-secondary)]">{item.description}</p>
            </div>
            <button
              type="button"
              className="pill mt-6 self-start"
              onClick={() => void start(item)}
              disabled={pending === item.id}
            >
              Открыть
            </button>
          </article>
        ))}
      </div>
    </main>
  );
}
