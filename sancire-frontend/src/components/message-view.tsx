import { memo, useEffect, useMemo, useRef, useState, type ReactNode, type ComponentProps } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import type { AuditReport, MessageMeta, ProcessStep, SourceRef } from "@/db/schema";
import { IconAlert, IconCheck, IconCopy, IconInfo, IconShield } from "@/components/icons";
import { authHeader } from "@/lib/chat-client";
import { useApp, type MessageRow } from "@/components/providers";

function TypingIndicator() {
  return (
    <div className="typing-tree" aria-label="Ассистент готовит ответ">
      <span className="typing-core" />
      <svg viewBox="0 0 72 44">
        <path className="b1" d="M36 22 C36 14, 26 12, 18 10" />
        <path className="b2" d="M36 22 C38 13, 50 12, 58 8" />
        <path className="b3" d="M36 22 C30 28, 22 30, 14 33" />
        <path className="b4" d="M36 22 C42 28, 52 30, 60 34" />
      </svg>
    </div>
  );
}
function SourceMark({
  source,
  children,
}: {
  source?: SourceRef;
  children: ReactNode;
}) {
  const { setReader } = useApp();
  const [open, setOpen] = useState(false);
  if (!source) return <span>{children}</span>;
  return (
    <span
      className="source-mark relative"
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onClick={() => setReader({ sectionId: source.id })}
    >
      {children}
      {open ? (
        <span className="source-pop left-0 top-[calc(100%+8px)] w-[260px]">
          <span className="block text-[13px] font-medium leading-snug">{source.documentTitle}</span>
          <span className="mt-1 block text-[12.5px] text-[var(--text-secondary)]">{source.clause}</span>
          <span className="mt-2 block text-[11.5px] text-[var(--text-secondary)]">
            Актуальная редакция от {source.updatedAt}
          </span>
        </span>
      ) : null}
    </span>
  );
}
function renderContent(content: string, sources: SourceRef[]) {
  // Keep the backend's source-reference markers as inline Markdown links, then
  // resolve them to the existing in-app source popover (no raw HTML is injected).
  const markdown = content.replace(/\{\{s:([^}]+)\}\}([\s\S]*?)\{\{\/s\}\}/g, (_all, id: string, text: string) =>
    `[${text.replace(/\n/g, " ")}](source:${encodeURIComponent(id)})`,
  );
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      urlTransform={(url) => url.startsWith("source:") ? url : defaultUrlTransform(url)}
      components={{
        a: ({ href, children, ...props }: ComponentProps<"a">) => {
          if (href?.startsWith("source:")) {
            const id = decodeURIComponent(href.slice("source:".length));
            const source = sources.find((item) => item.id === id);
            return <SourceMark source={source}>{children}</SourceMark>;
          }
          return <a href={href} target="_blank" rel="noreferrer" {...props}>{children}</a>;
        },
        table: ({ children }) => <div className="markdown-table-wrap"><table>{children}</table></div>,
      }}
    >
      {markdown}
    </ReactMarkdown>
  );
}
function ProcessDiagramView({ title = "Этапы процесса", steps }: { title?: string; steps: ProcessStep[] }) {
  const [openId, setOpenId] = useState<string | null>(null);
  const [hoverId, setHoverId] = useState<string | null>(null);
  const open = steps.find((step) => step.id === openId);
  return (
    <div className="process">
      <p className="mb-3 text-[13px] text-[var(--text-secondary)]">{title}</p>
      <div className="process-track">
        {steps.map((step, index) => (
          <button
            type="button"
            key={step.id}
            className="process-step"
            style={{ animationDelay: `${index * 120}ms` }}
            onMouseEnter={() => setHoverId(step.id)}
            onMouseLeave={() => setHoverId(null)}
            onClick={() => setOpenId(openId === step.id ? null : step.id)}
          >
            <span className="process-node" style={{ animationDelay: `${index * 120}ms` }} />
            {index < steps.length - 1 ? (
              <span className="process-line" style={{ animationDelay: `${index * 120 + 80}ms` }} />
            ) : null}
            <span className="process-label">{step.title}</span>
            {hoverId === step.id ? (
              <span className="step-pop top-[46px] z-20 w-[220px] text-left">
                {step.deadline ? (
                  <span className="block text-[12px] text-[var(--text-secondary)]">{step.deadline}</span>
                ) : null}
                {step.department ? (
                  <span className="mt-1 block text-[12.5px]">{step.department}</span>
                ) : null}
              </span>
            ) : null}
          </button>
        ))}
      </div>
      {open ? (
        <div className="mt-4 overflow-hidden rounded-[20px] bg-[var(--divider)] px-4 py-3 text-[14.5px] leading-relaxed">
          <div className="font-medium">{open.title}</div>
          <p className="mt-1 text-[var(--text-secondary)]">{open.description}</p>
        </div>
      ) : null}
    </div>
  );
}

function AuditCard({ audit }: { audit: AuditReport }) {
  const statusLabel =
    audit.status === "READY" ? "Готов к согласованию" : audit.status === "WARNINGS" ? "Есть предупреждения" : "Критические замечания";
  const visas = (audit.visaRouting?.requiredVisas || []).filter((visa) => visa.required);
  const rules = audit.visaRouting?.appliedRules || [];
  const findings = audit.findings || [];
  const shown = findings.slice(0, 6);
  const severityIcon = (severity: string) =>
    severity === "CRITICAL" ? <IconAlert size={14} /> : severity === "WARNING" ? <IconInfo size={14} /> : <IconCheck size={14} />;
  return (
    <div className={`audit-card audit-${audit.status === "READY" ? "green" : audit.status === "WARNINGS" ? "yellow" : "red"}`}>
      <div className="audit-card-head">
        <span className="audit-status-dot" />
        <span className="audit-status-label">{statusLabel}</span>
        {audit.fileName ? <span className="audit-file">{audit.fileName}</span> : null}
      </div>
      <div className="audit-counts">
        <span className="count crit">{audit.criticalCount ?? 0} критических</span>
        <span className="count warn">{audit.warningCount ?? 0} предупреждений</span>
        <span className="count info">{audit.infoCount ?? 0} рекомендаций</span>
      </div>
      {visas.length ? (
        <div className="audit-visas">
          <p className="audit-subtitle">
            <IconShield size={14} /> Маршрут визирования по Матрице
          </p>
          <ul>
            {visas.map((visa) => (
              <li key={visa.department}>
                <strong>{visa.department}</strong>
                {visa.reason ? <span> — {visa.reason}</span> : null}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {rules.length ? (
        <details className="audit-rules">
          <summary>Обоснование расчёта ({rules.length})</summary>
          <ul>
            {rules.map((rule, index) => (
              <li key={index}>{rule}</li>
            ))}
          </ul>
        </details>
      ) : null}
      {audit.visaRouting?.privilegeNote ? (
        <p className="audit-privilege">{audit.visaRouting.privilegeNote}</p>
      ) : null}
      {shown.length ? (
        <div className="audit-findings">
          <p className="audit-subtitle">Замечания</p>
          <ul>
            {shown.map((finding, index) => (
              <li key={index} className={`sev-${finding.severity.toLowerCase()}`}>
                <span className="sev-icon">{severityIcon(finding.severity)}</span>
                <span>
                  <strong>{finding.title}</strong>
                  {finding.location ? <em> · {finding.location}</em> : null}
                </span>
              </li>
            ))}
          </ul>
          {findings.length > shown.length ? (
            <p className="audit-more">… и ещё {findings.length - shown.length} — в отчёте</p>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

export type MessageViewProps = {
  message: MessageRow;
  isStreaming?: boolean;
  onFollowUp: (text: string) => void;
  onAction: (action: "explain" | "clarify" | "expert", selection?: string) => void;
};

function MessageViewBase({
  message,
  isStreaming = false,
  onFollowUp,
  onAction,
}: MessageViewProps) {
  const { showToast, setReader } = useApp();
  const rootRef = useRef<HTMLDivElement>(null);
  const [toolbar, setToolbar] = useState<{ x: number; y: number; text: string } | null>(null);
  const meta = message.meta;
  const sources = useMemo(() => meta?.sources ?? [], [meta]);

  useEffect(() => {
    function onSelect() {
      const selection = window.getSelection();
      if (!selection || selection.isCollapsed || !rootRef.current) {
        setToolbar(null);
        return;
      }
      if (!rootRef.current.contains(selection.anchorNode)) {
        setToolbar(null);
        return;
      }
      const text = selection.toString().trim();
      if (!text) {
        setToolbar(null);
        return;
      }
      const rect = selection.getRangeAt(0).getBoundingClientRect();
      const box = rootRef.current.getBoundingClientRect();
      setToolbar({
        x: rect.left - box.left + rect.width / 2,
        y: rect.top - box.top - 12,
        text,
      });
    }
    document.addEventListener("selectionchange", onSelect);
    return () => document.removeEventListener("selectionchange", onSelect);
  }, []);

  const staticContent = useMemo(() => {
    if (isStreaming) return null;
    return renderContent(message.content, sources);
  }, [message.content, sources, isStreaming]);

  if (message.role === "user") {
    return (
      <div className="message-enter mb-8 flex flex-col items-end gap-2">
        {meta?.files?.length ? (
          <div className="flex flex-wrap justify-end gap-2">
            {meta.files.map((file: any) => (
              <div key={file.name} className="file-chip">
                {file.name}
              </div>
            ))}
          </div>
        ) : null}
        <div className="user-bubble">{message.content}</div>
      </div>
    );
  }
  return (
    <div ref={rootRef} className="message-enter relative mb-10">
      <div className={`assistant-body ${isStreaming ? "is-streaming" : ""}`}>
        {isStreaming ? renderContent(message.content, sources) : staticContent}
        {isStreaming ? <span className="streaming-cursor" aria-hidden="true" /> : null}
      </div>
      {meta?.process ? <ProcessDiagramView title={meta.process.title} steps={meta.process.steps} /> : null}
      {meta?.audit ? <AuditCard audit={meta.audit} /> : null}
      {toolbar ? (
        <div
          className="mini-toolbar"
          style={{ left: toolbar.x, top: toolbar.y, transform: "translate(-50%, -100%)" }}
        >
          <button
            type="button"
            onClick={() => {
              void navigator.clipboard.writeText(toolbar.text);
              setToolbar(null);
              showToast("Скопировано");
            }}
          >
            <span className="inline-flex items-center gap-1">
              <IconCopy size={14} /> Скопировать
            </span>
          </button>
          <button
            type="button"
            onClick={() => {
              onAction("explain", toolbar.text);
              setToolbar(null);
            }}
          >
            Объяснить проще
          </button>
          <button
            type="button"
            onClick={() => {
              onAction("clarify", toolbar.text);
              setToolbar(null);
            }}
          >
            Уточнить
          </button>
        </div>
      ) : null}
    </div>
  );
}

export const MessageView = memo(MessageViewBase, (prev, next) => {
  return (
    prev.message.id === next.message.id &&
    prev.message.content === next.message.content &&
    prev.message.role === next.message.role &&
    prev.message.meta === next.message.meta &&
    prev.isStreaming === next.isStreaming &&
    prev.onFollowUp === next.onFollowUp &&
    prev.onAction === next.onAction
  );
});
export function AssistantPending() {
  return (
    <div className="message-enter mb-10">
      <TypingIndicator />
    </div>
  );
}
