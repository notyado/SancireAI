import type { AttachedFile } from "@/db/schema";
import type { ConversationRow, MessageRow } from "@/components/providers";

export type ChatResponse = { conversation: ConversationRow; created?: boolean; messages: MessageRow[] };
export type ChatPayload = {
  conversationId?: string;
  content: string;
  files?: AttachedFile[];
  action?: "explain" | "clarify" | "expert";
  selection?: string;
};

export function getAuthToken(): string | null {
  return typeof window === "undefined" ? null : localStorage.getItem("sancire_token");
}
export function setAuthToken(token: string) {
  if (typeof window !== "undefined") localStorage.setItem("sancire_token", token);
}
export function clearAuthToken() {
  if (typeof window !== "undefined") {
    localStorage.removeItem("sancire_token");
    localStorage.removeItem("sancire_user");
  }
}
function authHeaders(accept = "application/json"): Record<string, string> {
  const headers: Record<string, string> = { "Content-Type": "application/json", Accept: accept };
  const token = getAuthToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  return headers;
}

export async function createConversation(title?: string): Promise<{ id: string }> {
  const response = await fetch("/api/conversations", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify({ title: title || "Новый диалог" }),
  });
  if (!response.ok) throw new Error(`create_conv_failed_${response.status}`);
  return (await response.json()) as { id: string };
}

export async function sendChat(payload: ChatPayload): Promise<ChatResponse> {
  const response = await fetch("/api/chat", {
    method: "POST",
    headers: authHeaders(),
    body: JSON.stringify(payload),
  });
  if (!response.ok) throw new Error(`chat_failed_${response.status}`);
  return await response.json() as ChatResponse;
}


/**
 * Reads Server-Sent Events incrementally. `onStarted` fires as soon as the server
 * confirms it has persisted the user turn; callers must not retry the POST after that.
 */
export async function sendChatStream(
  payload: ChatPayload,
  onToken: (token: string) => void,
  onComplete: (data: ChatResponse) => void,
  onError?: (error: unknown, started: boolean) => void | Promise<void>,
  onStarted?: () => void,
  signal?: AbortSignal,
): Promise<void> {
  let started = false;
  let completed = false;
  const headers = authHeaders("text/event-stream");
  try {
    const response = await fetch("/api/chat?stream=true", {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
      signal,
    });
    if (!response.ok || !response.body) {
      const details = await response.text().catch(() => "");
      throw new Error(`stream_failed_${response.status}${details ? `: ${details}` : ""}`);
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    const consumeBlock = (block: string) => {
      if (!block.trim()) return;
      let event = "message";
      const dataLines: string[] = [];
      for (const line of block.split(/\r?\n/)) {
        if (!line || line.startsWith(":")) continue;
        const colon = line.indexOf(":");
        const field = colon < 0 ? line : line.slice(0, colon);
        const value = colon < 0 ? "" : line.slice(colon + 1).replace(/^ /, "");
        if (field === "event") event = value;
        if (field === "data") dataLines.push(value);
      }
      if (!dataLines.length) return;
      let parsed: Record<string, unknown>;
      try {
        parsed = JSON.parse(dataLines.join("\n")) as Record<string, unknown>;
      } catch {
        // Ignore a malformed event but keep draining the stream, so we don't
        // race a canonical history fetch against an unfinished server request.
        return;
      }
      if (event === "start") {
        if (!started) {
          started = true;
          onStarted?.();
        }
      } else if (event === "token" && typeof parsed.token === "string") {
        onToken(parsed.token);
      } else if (event === "complete") {
        completed = true;
        onComplete(parsed as unknown as ChatResponse);
      } else if (event === "error") {
        throw new Error(typeof parsed.message === "string" ? parsed.message : "stream_server_error");
      }
    };

    while (true) {
      if (signal?.aborted) {
        await reader.cancel().catch(() => {});
        return;
      }
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let boundary: number;
      while ((boundary = buffer.search(/\r?\n\r?\n/)) >= 0) {
        const separator = buffer.match(/\r?\n\r?\n/)![0];
        consumeBlock(buffer.slice(0, boundary));
        buffer = buffer.slice(boundary + separator.length);
      }
    }
    buffer += decoder.decode();
    if (buffer.trim()) consumeBlock(buffer);
    if (!completed && !signal?.aborted) throw new Error("stream_closed_before_complete");
  } catch (error: any) {
    if (signal?.aborted || error?.name === "AbortError") {
      return;
    }
    if (onError) await onError(error, started);
    else throw error;
  }
}
