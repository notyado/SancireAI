import { useEffect, useState } from "react";
import { IconExpand, IconX } from "@/components/icons";
import { useApp } from "@/components/providers";
import { authHeader } from "@/lib/chat-client";
type RegulationPayload = {
  regulation: {
    id: string;
    title: string;
    updatedAtLabel: string;
    intro: string;
    sections: { id: string; clause: string; title: string; text: string }[];
  } | null;
  section?: { id: string };
};
export function ReaderPanel() {
  const { reader, setReader, sidebarOpen } = useApp();
  const [data, setData] = useState<RegulationPayload | null>(null);
  const [failed, setFailed] = useState(false);
  const [modal, setModal] = useState(false);
  const open = Boolean(reader) && !sidebarOpen;
  useEffect(() => {
    if (!reader) {
      setData(null);
      setFailed(false);
      setModal(false);
      return;
    }
    let cancelled = false;
    setData(null);
    setFailed(false);
    void fetch(`/api/regulations?section=${encodeURIComponent(reader.sectionId)}`, { headers: authHeader() })
      .then((res) => (res.ok ? res.json() : Promise.reject(new Error(String(res.status)))))
      .then((payload: RegulationPayload) => {
        if (cancelled) return;
        setData(payload);
        // Let React paint the fragment before scrolling it into view.
        window.setTimeout(() => {
          document.getElementById(`para-${reader.sectionId}`)?.scrollIntoView({
            behavior: "smooth",
            block: "center",
          });
        }, 350);
      })
      .catch(() => {
        if (!cancelled) setFailed(true);
      });
    return () => {
      cancelled = true;
    };
  }, [reader]);
  const body = data?.regulation ? (
    <div className="px-5 pb-8">
      {data.regulation.updatedAtLabel ? (
        <p className="text-[12.5px] text-[var(--text-secondary)]">
          Актуальная редакция от {data.regulation.updatedAtLabel}
        </p>
      ) : null}
      <h2 className="mt-2 text-[22px] font-medium leading-snug tracking-[-0.03em]">{data.regulation.title}</h2>
      <p className="mt-4 text-[15px] text-[var(--text-secondary)]">{data.regulation.intro}</p>
      <div className="mt-6 space-y-5">
        {data.regulation.sections.map((section) => (
          <section
            key={section.id}
            id={`para-${section.id}`}
            className={section.id === reader?.sectionId ? "reg-paragraph active" : "reg-paragraph"}
          >
            <h3 className="text-[14px] font-medium">
              {section.clause}. {section.title}
            </h3>
            <p className="mt-2 text-[15px] leading-relaxed">{section.text}</p>
          </section>
        ))}
      </div>
    </div>
  ) : (
    <div className="px-5 text-[var(--text-secondary)]">
      {failed ? "Не удалось открыть источник — попробуйте ещё раз" : "Открываю документ…"}
    </div>
  );
  return (
    <>
      <aside className={`reader-panel ${open && !modal ? "open" : ""}`}>
        <div className="flex items-center justify-between px-4 pt-4">
          <span className="px-1 text-[13px] text-[var(--text-secondary)]">Источник</span>
          <div className="flex">
            <button type="button" className="icon-btn" onClick={() => setModal(true)} aria-label="Расширить">
              <IconExpand />
            </button>
            <button type="button" className="icon-btn" onClick={() => setReader(null)} aria-label="Закрыть">
              <IconX />
            </button>
          </div>
        </div>
        <div className="mt-2 flex-1 overflow-y-auto">{body}</div>
      </aside>
      {modal && reader ? (
        <div className="modal-layer" onClick={() => setModal(false)}>
          <div className="modal-card max-w-3xl" onClick={(event) => event.stopPropagation()}>
            <div className="mb-4 flex justify-end">
              <button type="button" className="icon-btn" onClick={() => setModal(false)} aria-label="Закрыть">
                <IconX />
              </button>
            </div>
            {body}
          </div>
        </div>
      ) : null}
    </>
  );
}
