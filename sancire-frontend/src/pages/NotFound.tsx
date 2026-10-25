import { Link } from "react-router-dom";
export default function NotFound() {
  return (
    <main className="grid min-h-screen place-items-center px-6">
      <div className="text-center">
        <p className="text-[36px] font-medium tracking-[-0.04em]">Страница не найдена</p>
        <p className="mt-3 text-[var(--text-secondary)]">Вернитесь к диалогу — там всё нужное.</p>
        <Link to="/" className="primary-btn solid mt-8 inline-flex px-5 py-2.5 text-sm">На главную</Link>
      </div>
    </main>
  );
}
