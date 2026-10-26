import { useCallback, useEffect, useRef, useState } from "react";
import { Composer } from "@/components/composer";
import { AssistantPending, MessageView } from "@/components/message-view";
import { useApp, type ConversationRow, type MessageRow } from "@/components/providers";
import { authHeader, sendChatStream } from "@/lib/chat-client";
import type { AttachedFile } from "@/db/schema";

type ChatAction = "explain" | "clarify" | "expert";
type AskExtra = { action?: ChatAction; selection?: string };

export function ChatView({ conversationId }: { conversationId: string }) {
  const { refreshConversations, showToast, setHeaderTitle } = useApp();
  const [conversation, setConversation] = useState<ConversationRow | null>(null);
  const [messages, setMessages] = useState<MessageRow[]>([]);
  const [pending, setPending] = useState(false);
  const [streamingId, setStreamingId] = useState<string | null>(null);
  const [missing, setMissing] = useState(false);
  const scroller = useRef<HTMLDivElement>(null);
  const pinnedToBottom = useRef(true);
  const initializedPrompt = useRef(false);
  const requestInFlight = useRef(false);
  const requestVersion = useRef(0);
  const rafId = useRef<number | null>(null);
  const streamedContentRef = useRef("");
  const streamingAsstIdRef = useRef<string | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);

  useEffect(() => {
    let ignore = false;
    setPending(false);
    setStreamingId(null);
    setMissing(false);
    setConversation(null);
    setMessages([]);
    initializedPrompt.current = false;
    requestInFlight.current = false;
    pinnedToBottom.current = true;
    const version = ++requestVersion.current;

    void fetch(`/api/conversations/${encodeURIComponent(conversationId)}`, { headers: authHeader() })
      .then(async (response) => {
        if (!response.ok) {
          if (!ignore) setMissing(true);
          return;
        }
        const data = (await response.json()) as { conversation: ConversationRow; messages: MessageRow[] };
        if (ignore || version !== requestVersion.current) return;
        setConversation(data.conversation);
        setMessages(data.messages);
        pinnedToBottom.current = true;

        const storageKey = `sancire_pending_prompt_${conversationId}`;
        const stored = sessionStorage.getItem(storageKey);
        if (stored && !initializedPrompt.current) {
          initializedPrompt.current = true;
          sessionStorage.removeItem(storageKey);
          try {
            const queued = JSON.parse(stored) as { text?: string; files?: AttachedFile[] };
            void ask(queued.text ?? "", queued.files ?? []);
          } catch (error) {
            console.error("Failed to parse pending prompt", error);
            showToast("Не удалось восстановить отправленное сообщение");
          }
        }
      })
      .catch(() => {
        if (!ignore) showToast("Не удалось загрузить диалог");
      });

    return () => {
      ignore = true;
      if (abortControllerRef.current) {
        abortControllerRef.current.abort();
        abortControllerRef.current = null;
      }
      if (rafId.current !== null) {
        cancelAnimationFrame(rafId.current);
        rafId.current = null;
      }
      ++requestVersion.current;
    };
    // `ask` deliberately isn't a dependency; the conversation load should run once per id.
  }, [conversationId]);

  useEffect(() => {
    setHeaderTitle(conversation?.title || "");
    return () => setHeaderTitle("");
  }, [conversation?.title, setHeaderTitle]);

  const flushStreamBatch = useCallback(() => {
    const asstId = streamingAsstIdRef.current;
    if (!asstId) return;
    const content = streamedContentRef.current;
    setMessages((previous) => {
      const assistant: MessageRow = {
        id: asstId,
        conversationId,
        role: "assistant",
        content,
        meta: null,
        createdAt: new Date().toISOString(),
      };
      const existing = previous.findIndex((item) => item.id === asstId);
      if (existing < 0) return [...previous, assistant];
      return previous.map((item) => (item.id === asstId ? assistant : item));
    });
  }, [conversationId]);

  useEffect(() => {
    const element = scroller.current;
    if (!element || !pinnedToBottom.current) return;
    element.scrollTop = element.scrollHeight;
  }, [messages, pending]);

  const handleScroll = useCallback(() => {
    const element = scroller.current;
    if (!element) return;
    pinnedToBottom.current = element.scrollHeight - element.scrollTop - element.clientHeight < 96;
  }, []);

  const stopGeneration = useCallback(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    if (rafId.current !== null) {
      cancelAnimationFrame(rafId.current);
      rafId.current = null;
    }
    streamingAsstIdRef.current = null;
    requestInFlight.current = false;
    setPending(false);
    setStreamingId(null);
    void refreshConversations();
  }, [refreshConversations]);

function safeRandomId(prefix: string): string {
  if (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function") {
    try {
      return `${prefix}-${crypto.randomUUID()}`;
    } catch {}
  }
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`;
}

  async function ask(text: string, files: AttachedFile[] = [], extra?: AskExtra) {
    if (requestInFlight.current) return;
    const cleanText = text.trim();
    if (!cleanText && files.length === 0 && extra?.action !== "expert") return;

    const localUserId = safeRandomId("local");
    const localAssistantId = safeRandomId("stream");
    const userContent = cleanText || (files[0] ? `Проверьте документ: ${files[0].name}` : "Вопрос эксперту");
    const localUser: MessageRow = {
      id: localUserId,
      conversationId,
      role: "user",
      content: userContent,
      meta: files.length ? { files } : null,
      createdAt: new Date().toISOString(),
    };

    requestInFlight.current = true;
    pinnedToBottom.current = true;
    setPending(true);
    setStreamingId(localAssistantId);
    streamingAsstIdRef.current = localAssistantId;
    streamedContentRef.current = "";
    setMessages((previous) => [...previous.filter((item) => item.id !== localUserId), localUser]);

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    let acceptedByServer = false;
    try {
      await sendChatStream(
        { conversationId, content: cleanText, files, action: extra?.action, selection: extra?.selection },
        (token) => {
          streamedContentRef.current += token;
          if (rafId.current === null) {
            rafId.current = requestAnimationFrame(() => {
              rafId.current = null;
              flushStreamBatch();
            });
          }
        },
        (result) => {
          if (rafId.current !== null) {
            cancelAnimationFrame(rafId.current);
            rafId.current = null;
          }
          acceptedByServer = true;
          setConversation(result.conversation);
          setMessages(deduplicateMessages(result.messages));
          void refreshConversations();
        },
        async (error, started) => {
          if (abortController.signal.aborted) return;
          if (rafId.current !== null) {
            cancelAnimationFrame(rafId.current);
            rafId.current = null;
          }
          acceptedByServer = acceptedByServer || started;
          console.warn("Chat stream ended before completion", error);
          try {
            const response = await fetch(`/api/conversations/${encodeURIComponent(conversationId)}`, {
              headers: authHeader(),
            });
            if (response.ok) {
              const canonical = await response.json() as { conversation: ConversationRow; messages: MessageRow[] };
              setConversation(canonical.conversation);
              const serverMessages = deduplicateMessages(canonical.messages);
              if (serverMessages.some((message) => message.id === localUserId) || acceptedByServer) {
                setMessages(serverMessages);
              } else {
                setMessages((previous) => previous.filter((message) => message.id !== localAssistantId));
              }
              void refreshConversations();
            } else if (streamedContentRef.current) {
              setMessages((previous) => previous.filter((message) => message.id !== localAssistantId));
            }
          } catch {
            if (!streamedContentRef.current) {
              setMessages((previous) => previous.filter((message) => message.id !== localAssistantId));
            }
          }
          showToast("Ответ прервался. История диалога синхронизирована; повторите запрос при необходимости.");
        },
        () => { acceptedByServer = true; },
        abortController.signal,
      );
    } catch (error: any) {
      if (abortController.signal.aborted || error?.name === "AbortError") {
        return;
      }
      console.error("Unable to send chat turn", error);
      showToast("Не удалось отправить сообщение");
    } finally {
      if (abortControllerRef.current === abortController) {
        abortControllerRef.current = null;
      }
      if (rafId.current !== null) {
        cancelAnimationFrame(rafId.current);
        rafId.current = null;
      }
      streamingAsstIdRef.current = null;
      requestInFlight.current = false;
      setPending(false);
      setStreamingId(null);
    }
  }

  const askRef = useRef(ask);
  askRef.current = ask;

  const handleFollowUp = useCallback((followUp: string) => {
    void askRef.current(followUp);
  }, []);

  const handleAction = useCallback((action: ChatAction, selection?: string) => {
    const text =
      action === "explain" ? "Объяснить проще" : action === "clarify" ? "Уточнить" : "Задать вопрос эксперту";
    void askRef.current(text, [], { action, selection });
  }, []);

  const handleComposerSubmit = useCallback(({ text, files }: { text: string; files: AttachedFile[] }) => {
    void askRef.current(text, files);
  }, []);

  if (missing) {
    return <main className="grid min-h-screen place-items-center px-6"><p className="text-[var(--text-secondary)]">Этот диалог больше недоступен.</p></main>;
  }

  return (
    <main className="chat-page page-fade flex h-screen h-[100dvh] w-full flex-col overflow-hidden">
      <div
        ref={scroller}
        onScroll={handleScroll}
        className="chat-messages min-h-0 flex-1 w-full overflow-y-auto"
        aria-live="polite"
        aria-relevant="additions text"
      >
        <div className="mx-auto flex w-full max-w-[840px] flex-col px-5 pb-8 pt-24 md:px-8">
          {messages.map((message, index) => (
            <div key={message.id} style={{ animationDelay: `${Math.min(index, 6) * 60}ms` }}>
              <MessageView
                message={message}
                isStreaming={message.id === streamingId}
                onFollowUp={handleFollowUp}
                onAction={handleAction}
              />
            </div>
          ))}
          {pending && !messages.some((message) => message.id === streamingId) ? <AssistantPending /> : null}
        </div>
      </div>
      <div className="shrink-0 w-full pb-6 pt-2">
        <div className="mx-auto w-full max-w-[840px] px-5 md:px-8">
          {pending ? (
            <div className="stop-generation-bar">
              <button
                type="button"
                className="stop-generation-btn"
                onClick={stopGeneration}
                aria-label="Прервать ответ"
              >
                <span className="stop-icon" />
                Прервать ответ
              </button>
            </div>
          ) : null}
          <Composer
            disabled={pending}
            onStop={stopGeneration}
            onSubmit={handleComposerSubmit}
          />
        </div>
      </div>
    </main>
  );
}

function deduplicateMessages(messages: MessageRow[]): MessageRow[] {
  const seen = new Set<string>();
  return messages.filter((message) => {
    if (seen.has(message.id)) return false;
    seen.add(message.id);
    return true;
  });
}
