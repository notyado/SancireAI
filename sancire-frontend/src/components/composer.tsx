import { useEffect, useRef, useState, type DragEvent, type FormEvent } from "react";
import { IconMic, IconPaperclip, IconSend, IconStop, IconX } from "@/components/icons";
import { useApp } from "@/components/providers";
import { MAX_DOCX_BYTES } from "@/lib/chat-client";
export type ComposerFile = {
  name: string;
  size: number;
  type: string;
  /** base64 payload attached for .docx (chat audit flow) */
  data?: string;
};

/** Reads a file as base64 (no data: prefix) for chat attachments. */
function readAsBase64(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error);
    reader.onload = () => {
      const result = String(reader.result || "");
      const comma = result.indexOf(",");
      resolve(comma >= 0 ? result.slice(comma + 1) : result);
    };
    reader.readAsDataURL(file);
  });
}
type Props = {
  variant?: "hero" | "dock";
  placeholder?: string;
  disabled?: boolean;
  onStop?: () => void;
  onSubmit: (payload: { text: string; files: ComposerFile[] }) => Promise<void> | void;
};
export function Composer({
  variant = "dock",
  placeholder = "Задайте вопрос о регламенте...",
  disabled,
  onStop,
  onSubmit,
}: Props) {
  const [text, setText] = useState("");
  const [files, setFiles] = useState<(ComposerFile & { raw?: File })[]>([]);
  const [focus, setFocus] = useState(false);
  const [dragging, setDragging] = useState(false);
  const [sending, setSending] = useState(false);
  const { showToast } = useApp();
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    const el = areaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 180)}px`;
  }, [text]);
  // Attachments are read into base64 only when the message is sent, so a chip
  // is added as-is: no upload starts here and none has to be simulated.
  function addFiles(list: FileList | File[]) {
    const incoming = Array.from(list).map((file) => ({
      name: file.name,
      size: file.size,
      type: file.type,
      raw: file,
    }));
    setFiles((prev) => [...prev, ...incoming]);
  }
  function toggleVoice() {
    showToast("Голосовой ввод пока в разработке — пользуйтесь клавиатурой");
  }

  async function submit(event?: FormEvent) {
    event?.preventDefault();
    const payload = text.trim();
    if ((!payload && files.length === 0) || disabled) return;
    setSending(true);
    try {
      const outgoing: ComposerFile[] = [];
      for (const { name, size, type, raw } of files) {
        const entry: ComposerFile = { name, size, type };
        if (raw && name.toLowerCase().endsWith(".docx") && size <= MAX_DOCX_BYTES) {
          try {
            entry.data = await readAsBase64(raw);
          } catch {
            /* attachment without payload — backend will ask to re-upload */
          }
        }
        outgoing.push(entry);
      }
      await onSubmit({ text: payload, files: outgoing });
    } finally {
      setSending(false);
    }
    setText("");
    setFiles([]);
    if (areaRef.current) {
      areaRef.current.style.height = "auto";
    }
  }
  const ready = Boolean(text.trim() || files.length);
  return (
    <form
      onSubmit={submit}
      onDragOver={(event: DragEvent) => {
        event.preventDefault();
        setDragging(true);
      }}
      onDragLeave={() => setDragging(false)}
      onDrop={(event: DragEvent) => {
        event.preventDefault();
        setDragging(false);
        if (event.dataTransfer.files.length) addFiles(event.dataTransfer.files);
      }}
      className={`composer-shell ${variant} ${focus ? "focus" : ""} ${disabled ? "busy" : ""} ${sending ? "sending" : ""}`}
    >
      {dragging ? <div className="drop-overlay">Отпустите файл здесь</div> : null}
      {files.length > 0 ? (
        <div className="mb-2 flex gap-2 overflow-x-auto px-1 pt-1">
          {files.map((file) => (
            <div key={file.name} className="file-chip group relative shrink-0">
              <span className="grid h-8 w-8 place-items-center rounded-full border border-[var(--divider)] text-[12px] text-[var(--accent)]">
                ✓
              </span>
              <span className="max-w-[140px] truncate">{file.name}</span>
              <button
                type="button"
                className="absolute -right-1 -top-1 hidden h-5 w-5 place-items-center rounded-full bg-[var(--surface-solid)] group-hover:grid"
                onClick={() => setFiles((prev) => prev.filter((item) => item.name !== file.name))}
                aria-label="Удалить файл"
              >
                <IconX size={12} />
              </button>
            </div>
          ))}
        </div>
      ) : null}
      <div className="flex items-end gap-1">
        <button
          type="button"
          className="icon-btn"
          onClick={() => fileRef.current?.click()}
          aria-label="Прикрепить файл"
        >
          <IconPaperclip />
        </button>
        <input
          ref={fileRef}
          type="file"
          className="hidden"
          multiple
          onChange={(event) => {
            if (event.target.files) addFiles(event.target.files);
            event.target.value = "";
          }}
        />
        <textarea
          ref={areaRef}
          rows={1}
          value={text}
          placeholder={placeholder}
          onChange={(event) => setText(event.target.value)}
          onFocus={() => setFocus(true)}
          onBlur={() => setFocus(false)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              void submit();
            }
          }}
        />
        <button
          type="button"
          className="icon-btn"
          onClick={toggleVoice}
          aria-label="Голосовой ввод"
          title="Голосовой ввод (в разработке)"
        >
          <IconMic />
        </button>
        {disabled && onStop ? (
          <button
            type="button"
            className="send-btn ready stop"
            onClick={onStop}
            aria-label="Остановить генерацию"
            title="Остановить генерацию"
          >
            <IconStop size={14} />
          </button>
        ) : (
          <button
            type="submit"
            className={`send-btn ${ready ? "ready" : ""}`}
            aria-label="Отправить"
            disabled={!ready || disabled}
          >
            <IconSend />
          </button>
        )}
      </div>
    </form>
  );
}
