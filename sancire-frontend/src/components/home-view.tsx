"use client";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Composer } from "@/components/composer";
import { AssistantPending } from "@/components/message-view";
import { useApp } from "@/components/providers";
import { createConversation } from "@/lib/chat-client";
import { greetingByHour } from "@/lib/dates";
const SUGGESTIONS = [
  "Приказ о вводе ИС в эксплуатацию",
  "Как согласовать внутренний документ?",
  "Проверить готовый документ",
  "Как оформить реестр замечаний?",
];
export function HomeView() {
  const navigate = useNavigate();
  const { refreshConversations, showToast } = useApp();
  const [hello, setHello] = useState("Добрый день");
  const [pending, setPending] = useState(false);
  useEffect(() => {
    setHello(greetingByHour(new Date().getHours()));
  }, []);
  async function ask(text: string, files: { name: string; size: number; type: string }[] = []) {
    if (!text.trim() && files.length === 0) return;
    setPending(true);
    try {
      const title = text.slice(0, 45).trim() || (files[0] ? files[0].name : "Новый диалог");
      const conv = await createConversation(title);
      if (typeof window !== "undefined") {
        sessionStorage.setItem(
          `sancire_pending_prompt_${conv.id}`,
          JSON.stringify({ text, files }),
        );
      }
      await refreshConversations();
      navigate(`/chat/${conv.id}`);
    } catch {
      setPending(false);
      showToast("Не удалось создать диалог — попробуйте ещё раз");
    }
  }
  return (
    <main className="page-fade relative mx-auto flex min-h-screen w-full max-w-[840px] flex-col items-center justify-center px-6 py-24 md:px-10">
      {pending ? (
        <div className="flex flex-col items-center gap-6">
          <AssistantPending />
        </div>
      ) : (
        <>
          <p className="text-center text-[40px] font-medium leading-[1.1] tracking-[-0.045em] md:text-[48px]">
            {hello}
          </p>
          <p className="mt-3 text-center text-[18px] tracking-[-0.02em] text-[var(--text-secondary)]">
            Чем помочь?
          </p>
          <p className="mt-5 max-w-[520px] text-center text-[15.5px] leading-relaxed text-[var(--text-secondary)]">
            Спросите про порядок согласования, сроки или найдите нужный шаблон документа
          </p>
          <div className="mt-10 w-full">
            <Composer variant="hero" onSubmit={({ text, files }) => ask(text, files)} />
          </div>
          <div className="mt-6 flex flex-wrap justify-center gap-2">
            {SUGGESTIONS.map((item) => (
              <button key={item} type="button" className="pill" onClick={() => void ask(item)}>
                {item}
              </button>
            ))}
          </div>
        </>
      )}
    </main>
  );
}
