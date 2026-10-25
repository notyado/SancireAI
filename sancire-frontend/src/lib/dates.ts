export function startOfDay(date: Date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}
export function conversationGroup(dateInput: Date | string) {
  const date = typeof dateInput === "string" ? new Date(dateInput) : dateInput;
  const today = startOfDay(new Date());
  const target = startOfDay(date);
  const diff = Math.round((today.getTime() - target.getTime()) / 86_400_000);
  if (diff <= 0) return "Сегодня";
  if (diff === 1) return "Вчера";
  if (diff < 7) return "На прошлой неделе";
  return "Ранее";
}
export function greetingByHour(hour: number) {
  if (hour >= 5 && hour < 12) return "Доброе утро";
  if (hour >= 12 && hour < 18) return "Добрый день";
  return "Добрый вечер";
}
export function formatShortDate(value: string) {
  const date = new Date(value);
  return date.toLocaleDateString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
  });
}
export function truncateTitle(text: string, max = 52) {
  const clean = text.replace(/\s+/g, " ").trim();
  if (clean.length <= max) return clean || "Новый диалог";
  return `${clean.slice(0, max - 1).trim()}…`;
}
