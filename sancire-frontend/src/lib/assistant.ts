import type { ConversationDraft, GeneratedDocument, MessageMeta, SourceRef } from "@/db/schema";
import { allSections, findSection, formatUpdatedAt } from "@/lib/regulations";
type HistoryItem = {
  role: string;
  content: string;
};
export type AssistantInput = {
  text: string;
  files?: { name: string; type: string; size: number }[];
  history: HistoryItem[];
  draft: ConversationDraft;
  action?: "explain" | "clarify" | "expert";
  selection?: string;
};
export type AssistantResult = {
  content: string;
  meta: MessageMeta;
  draft: ConversationDraft;
};
type ScoredSection = (typeof allSections)[number] & { score: number };
const TEMPLATE_FLOWS: Record<
  string,
  {
    id: string;
    title: string;
    start: RegExp;
    questions: { field: string; prompt: string }[];
    build: (fields: Record<string, string>) => GeneratedDocument;
  }
> = {
  memo: {
    id: "memo",
    title: "служебную записку",
    start: /служебн(ую|ая|ой)\s+записк|служебк|оформи.*записк|шаблон.*записк/,
    questions: [
      {
        field: "addressee",
        prompt: "Кому адресована записка? Укажите должность и подразделение.",
      },
      {
        field: "subject",
        prompt: "Сформулируйте тему — коротко, как в шапке документа.",
      },
      {
        field: "body",
        prompt: "Опишите суть обращения и какой результат вы ожидаете.",
      },
    ],
    build: (fields) => {
      const date = new Date().toLocaleDateString("ru-RU");
      const body = `СЛУЖЕБНАЯ ЗАПИСКА
Кому: ${fields.addressee}
Дата: ${date}
Тема: ${fields.subject}
${fields.body}
Прошу рассмотреть и принять решение.`;
      return {
        title: `Служебная записка — ${fields.subject}`,
        kind: "Служебная записка",
        preview: `${fields.subject}. Адресат: ${fields.addressee}.`,
        body,
      };
    },
  },
  trip: {
    id: "trip",
    title: "служебное задание на командировку",
    start: /служебное задание|оформи.*командир|шаблон.*командир/,
    questions: [
      {
        field: "destination",
        prompt: "Куда планируется командировка и на какие даты?",
      },
      {
        field: "purpose",
        prompt: "Какова цель поездки — что нужно сделать на месте?",
      },
    ],
    build: (fields) => {
      const body = `СЛУЖЕБНОЕ ЗАДАНИЕ НА КОМАНДИРОВКУ
Место и сроки: ${fields.destination}
Цель: ${fields.purpose}
Прошу согласовать направление в командировку.`;
      return {
        title: "Служебное задание на командировку",
        kind: "Служебное задание",
        preview: fields.destination,
        body,
      };
    },
  },
  leave: {
    id: "leave",
    title: "заявление на отпуск",
    start: /заявлен.*отпуск|оформи.*отпуск|шаблон.*отпуск/,
    questions: [
      {
        field: "dates",
        prompt: "Укажите даты отпуска — с какого по какое число.",
      },
    ],
    build: (fields) => {
      const body = `ЗАЯВЛЕНИЕ
Прошу предоставить ежегодный оплачиваемый отпуск ${fields.dates}.`;
      return {
        title: "Заявление на отпуск",
        kind: "Заявление",
        preview: fields.dates,
        body,
      };
    },
  },
};
function normalize(value: string) {
  return value
    .toLowerCase()
    .replace(/ё/g, "е")
    .replace(/[^\p{L}\p{N}\s-]/gu, " ")
    .replace(/\s+/g, " ")
    .trim();
}
function tokenize(value: string) {
  return normalize(value)
    .split(" ")
    .filter((token) => token.length > 2);
}
function mark(sectionId: string, text: string) {
  return `{{s:${sectionId}}}${text}{{/s}}`;
}
function sourceFromSection(section: {
  id: string;
  shortTitle: string;
  clause: string;
  text: string;
  updatedAt: string;
}): SourceRef {
  return {
    id: section.id,
    documentTitle: section.shortTitle,
    clause: section.clause,
    excerpt: section.text,
    updatedAt: formatUpdatedAt(section.updatedAt),
    paragraphId: section.id,
  };
}
function scoreSections(query: string): ScoredSection[] {
  const tokens = tokenize(query);
  if (!tokens.length) return [];
  return allSections
    .map((section) => {
      const hay = normalize(
        `${section.title} ${section.text} ${section.keywords.join(" ")} ${section.clause} ${section.shortTitle}`,
      );
      let score = 0;
      for (const token of tokens) {
        if (hay.includes(token)) score += token.length > 5 ? 2 : 1;
      }
      for (const keyword of section.keywords) {
        if (normalize(query).includes(normalize(keyword))) score += 4;
      }
      return { ...section, score };
    })
    .filter((section) => section.score > 0)
    .sort((a, b) => b.score - a.score);
}
function followUpsFor(kind: string): string[] {
  switch (kind) {
    case "approval":
      return [
        "Какие сроки на каждом этапе?",
        "Как оформить лист согласования?",
        "Что делать при разногласиях?",
      ];
    case "trip":
      return ["Как сдать авансовый отчёт?", "Какие суточные положены?", "Оформить служебное задание"];
    case "leave":
      return ["Как перенести отпуск?", "Оформить заявление на отпуск"];
    case "memo":
      return ["Какой срок ответа на записку?", "Оформить служебную записку", "Нужен лист согласования?"];
    case "security":
      return ["Что делать при инциденте?", "Можно ли отправить файл на личную почту?"];
    case "pdn":
      return ["Когда нужно отдельное согласие?", "Как передать данные контрагенту?"];
    case "procurement":
      return ["Как согласовать договор с поставщиком?", "Что указать в заявке на закупку?"];
    case "notFound":
      return ["Порядок согласования документа", "Оформить служебную записку", "Задать вопрос эксперту"];
    default:
      return ["Порядок согласования", "Сроки командировки", "Открыть библиотеку шаблонов"];
  }
}
function detectTemplate(text: string) {
  const n = normalize(text);
  return Object.values(TEMPLATE_FLOWS).find((flow) => flow.start.test(n));
}
function looksLikeNewQuestion(text: string) {
  const n = normalize(text);
  if (n.length < 12) return false;
  return /(как|какой|какая|какие|зачем|почему|нужно ли|можно ли|срок|согласован|командир|отпуск|регламент|инструкц)/.test(
    n,
  );
}
function buildGenericAnswer(sections: ScoredSection[], query: string): AssistantResult {
  const top = sections.slice(0, 3);
  const sources = top.map(sourceFromSection);
  const first = top[0];
  const second = top[1];
  const lines = [
    `По вашему вопросу нашлось в «${first.shortTitle}».`,
    "",
    mark(first.id, first.text),
  ];
  if (second && second.score >= 3) {
    lines.push("", mark(second.id, second.text));
  }
  if (/прост|понятн|объясн/.test(normalize(query))) {
    lines.unshift("Если коротко: главное — следовать указанному пункту и не пропускать обязательные шаги.", "");
  }
  return {
    content: lines.join("\n"),
    meta: {
      sources,
      followUps: [
        "Объяснить проще",
        `Открыть ${first.shortTitle}`,
        "Нужен шаблон документа",
      ],
    },
    draft: null,
  };
}
function approvalAnswer(): AssistantResult {
  const s1 = findSection("approval-2-1")!;
  const s2 = findSection("approval-3-1")!;
  const s3 = findSection("docflow-4-2")!;
  return {
    content: [
      "Согласование внутреннего документа идёт по фиксированному маршруту — его удобнее смотреть схемой, чем списком пунктов.",
      "",
      "Исполнитель готовит проект, пояснительную записку и лист согласования, затем документ движется по визам. Пропускать обязательного визирующего нельзя.",
      "",
      mark(s3.id, s3.text),
      "",
      mark(s2.id, s2.text),
    ].join("\n"),
    meta: {
      sources: [s3, s2, s1].map(sourceFromSection),
      process: {
        title: "Маршрут согласования внутреннего документа",
        steps: [
          {
            id: "prep",
            title: "Подготовка проекта",
            description:
              "Исполнитель готовит проект, пояснительную записку и лист согласования. При изменении действующего акта прилагается сравнительная таблица.",
            deadline: "До направления по маршруту",
            department: "Подразделение-инициатор",
          },
          {
            id: "legal",
            title: "Согласование с юр. департаментом",
            description:
              "Проверка на соответствие законодательству и внутренним актам. Замечания — с указанием пункта и предлагаемой редакцией.",
            deadline: "До 5 рабочих дней",
            department: "Юридический департамент",
          },
          {
            id: "curator",
            title: "Визирование куратором",
            description: "Куратор направления подтверждает целесообразность и снимает разногласия, если они остались.",
            deadline: "До 3 рабочих дней",
            department: "Куратор направления",
          },
          {
            id: "sign",
            title: "Подписание руководителем",
            description: "После сбора всех виз проект передаётся уполномоченному руководителю.",
            deadline: "После полного комплекта виз",
            department: "Уполномоченный руководитель",
          },
          {
            id: "reg",
            title: "Регистрация",
            description: "Документ регистрируется в системе электронного документооборота в течение одного рабочего дня.",
            deadline: "1 рабочий день",
            department: "Служба делопроизводства",
          },
        ],
      },
      followUps: followUpsFor("approval"),
    },
    draft: null,
  };
}
function tripAnswer(query: string): AssistantResult {
  const s1 = findSection("trips-2-2")!;
  const s2 = findSection("trips-4-2")!;
  const s3 = findSection("trips-1-3")!;
  const aboutReport = /отчет|отчёт|вернул/.test(normalize(query));
  const aboutPay = /суточн|расход|аванс/.test(normalize(query));
  if (aboutPay) {
    const pay = findSection("trips-3-1")!;
    return {
      content: [
        "По расходам в командировке действует простое правило: суточные — за каждый день, включая путь, проживание и проезд — по факту при наличии документов.",
        "",
        mark(pay.id, pay.text),
      ].join("\n"),
      meta: { sources: [sourceFromSection(pay)], followUps: followUpsFor("trip") },
      draft: null,
    };
  }
  return {
    content: [
      aboutReport
        ? "После возвращения остаётся короткий отчётный шаг — его легко пропустить, если не держать срок перед глазами."
        : "Командировка начинается не с билетов, а со служебного задания. Без него приказ не издаётся.",
      "",
      mark(s3.id, s3.text),
      "",
      mark(aboutReport ? s2.id : s1.id, aboutReport ? s2.text : s1.text),
    ].join("\n"),
    meta: {
      sources: [s3, aboutReport ? s2 : s1].map(sourceFromSection),
      process: {
        title: "Оформление командировки",
        steps: [
          {
            id: "task",
            title: "Служебное задание",
            description: "Готовится задание с целью, местом и сроками. Без него приказ не издаётся.",
            deadline: "До согласования",
            department: "Работник и руководитель",
          },
          {
            id: "boss",
            title: "Согласование руководителя",
            description: "Непосредственный руководитель согласовывает, руководитель подразделения утверждает.",
            deadline: "За 3 рабочих дня до выезда по РФ",
            department: "Структурное подразделение",
          },
          {
            id: "order",
            title: "Приказ о командировании",
            description: "Кадровая служба издаёт приказ на основании утверждённого задания.",
            deadline: "После утверждения задания",
            department: "Подразделение кадров",
          },
          {
            id: "report",
            title: "Авансовый отчёт",
            description: "Отчёт, проездные, счета за проживание и задание с отметкой о выполнении.",
            deadline: "3 рабочих дня после возвращения",
            department: "Бухгалтерия",
          },
        ],
      },
      followUps: followUpsFor("trip"),
    },
    draft: null,
  };
}
function checkDocument(files: { name: string; type: string; size: number }[], text: string): AssistantResult {
  const name = files[0]?.name ?? "документ";
  const s1 = findSection("docflow-3-1")!;
  const s2 = findSection("docflow-2-3")!;
  const s3 = findSection("docflow-4-2")!;
  const isMemo = /записк|memo|служеб/i.test(name + text);
  const isContract = /договор|контракт|contract/i.test(name + text);
  const issues = isContract
    ? [
        "В проекте не видно отметки о прохождении юридического согласования — без этой визы подписание преждевременно.",
        "Существенные условия после визирования менять нельзя без повторного круга.",
      ]
    : [
        isMemo
          ? "В шапке стоит проверить адресата: должность и подразделение должны быть указаны полностью, без инициалов подразделения."
          : "Тема документа должна совпадать с содержанием — сейчас по названию файла это неочевидно.",
        "Не видно листа согласования. Если документ не оперативный, маршрут визирования обязателен.",
        "Имеет смысл явно сформулировать ожидаемое решение в финальном абзаце — так адресату проще ответить в срок.",
      ];
  return {
    content: [
      `Проверила «${name}» по требованиям к внутренним документам. Критичных запретов нет — есть несколько мест, которые лучше поправить до отправки.`,
      "",
      issues.map((item, index) => `${index + 1}. ${item}`).join("\n"),
      "",
      mark(s1.id, s1.text),
      "",
      mark(s3.id, s3.text),
    ].join("\n"),
    meta: {
      sources: [s1, s3, s2].map(sourceFromSection),
      files,
      followUps: ["Оформить лист согласования", "Как согласовать документ?", "Подготовить сопроводительную записку"],
    },
    draft: null,
  };
}
function continueDraft(text: string, draft: NonNullable<ConversationDraft>): AssistantResult {
  if (!draft?.templateId || draft.step === undefined) {
    return {
      content: "Черновик шаблона не найден. Начните заново — например, «оформить служебную записку».",
      meta: { followUps: followUpsFor("memo") },
      draft: null,
    };
  }
  const flow = TEMPLATE_FLOWS[draft.templateId];
  if (!flow) {
    return {
      content: "Черновик шаблона не найден. Начните заново — например, «оформить служебную записку».",
      meta: { followUps: followUpsFor("memo") },
      draft: null,
    };
  }
  const field = flow.questions[draft.step]?.field;
  const fields = { ...draft.fields, ...(field ? { [field]: text.trim() } : {}) };
  const nextStep = draft.step + 1;
  if (nextStep < flow.questions.length) {
    return {
      content: `Приняла. ${flow.questions[nextStep].prompt}`,
      meta: {
        clarifying: true,
        followUps: ["Отмена"],
      },
      draft: { templateId: flow.id, step: nextStep, fields },
    };
  }
  const document = flow.build(fields);
  const s = findSection("docflow-3-1");
  return {
    content: [
      "Черновик готов. Проверьте адресата и формулировку темы — после этого документ можно скачать или отправить на согласование.",
      "",
      s ? mark(s.id, "Тема должна быть краткой и совпадать с содержанием. В конце стоит явно сформулировать просьбу.") : "",
    ]
      .filter(Boolean)
      .join("\n"),
    meta: {
      document,
      sources: s ? [sourceFromSection(s)] : [],
      followUps: ["Отправить на согласование", "Изменить адресата", "Оформить ещё один документ"],
    },
    draft: null,
  };
}
function startDraft(flowId: string): AssistantResult {
  const flow = TEMPLATE_FLOWS[flowId];
  return {
    content: `Подготовлю ${flow.title} в диалоге — без лишних полей. ${flow.questions[0].prompt}`,
    meta: {
      clarifying: true,
      followUps: ["Отмена", "Открыть библиотеку шаблонов"],
    },
    draft: { templateId: flow.id, step: 0, fields: {} },
  };
}
function notFound(): AssistantResult {
  return {
    content: [
      "В доступных регламентах нет достаточно точного ответа на этот вопрос — лучше не додумывать формулировку.",
      "",
      "Можно уточнить запрос (вид документа, подразделение, срок) или передать вопрос эксперту методологии. Если речь о шаблоне — его можно собрать в диалоге.",
    ].join("\n"),
    meta: {
      notFound: true,
      followUps: followUpsFor("notFound"),
    },
    draft: null,
  };
}
function expertAnswer(): AssistantResult {
  return {
    content:
      "Вопрос передан эксперту отдела методологии. Ответ придёт в эту переписку — обычно в течение рабочего дня. Ничего дополнительно оформлять не нужно.",
    meta: {
      expert: true,
      followUps: ["Порядок согласования", "Оформить служебную записку"],
    },
    draft: null,
  };
}
export function generateAssistantReply(input: AssistantInput): AssistantResult {
  const raw = input.text.trim();
  const n = normalize(raw);
  const lastUser = [...input.history].reverse().find((item) => item.role === "user")?.content ?? "";
  const contextQuery = `${lastUser} ${raw}`;
  if (input.action === "expert" || /задать вопрос эксперту|передайте эксперту|нужен эксперт/.test(n)) {
    return expertAnswer();
  }
  if (input.action === "explain" || (input.selection && /объяснить проще|проще/.test(n))) {
    const selection = input.selection || raw;
    return {
      content: [
        "Простыми словами:",
        "",
        selection.length > 400 ? `${selection.slice(0, 400).trim()}…` : selection,
        "",
        "Имеется в виду обязательное правило, а не рекомендация. Если шаг в маршруте пропущен, документ придётся возвращать на доработку.",
      ].join("\n"),
      meta: { followUps: ["Показать источник", "Что будет, если пропустить шаг?"] },
      draft: input.draft,
    };
  }
  if (input.action === "clarify" && input.selection) {
    return {
      content: `Уточните, пожалуйста, что именно нужно разобрать в этом фрагменте: срок, ответственное подразделение или как оформить документ?\n\n«${input.selection.slice(0, 220)}»`,
      meta: {
        clarifying: true,
        followUps: ["Срок", "Кто отвечает", "Нужен шаблон"],
      },
      draft: input.draft,
    };
  }
  if (input.files && input.files.length > 0) {
    return checkDocument(input.files, raw);
  }
  if (input.draft && /отмена|не надо|стоп/.test(n)) {
    return {
      content: "Хорошо, черновик закрыла. Можно задать другой вопрос по регламенту.",
      meta: { followUps: followUpsFor("default") },
      draft: null,
    };
  }
  if (input.draft && !looksLikeNewQuestion(raw)) {
    return continueDraft(raw, input.draft);
  }
  const template = detectTemplate(n);
  if (template) {
    return startDraft(template.id);
  }
  if (/согласован|визир|маршрут|утверд.*документ|как.*документ/.test(n) && /документ|положен|регламент|инструкц|внутрен/.test(n + " документ")) {
    return approvalAnswer();
  }
  if (/согласован|лист согласования|виза/.test(n)) {
    return approvalAnswer();
  }
  if (/командир/.test(n)) {
    return tripAnswer(raw);
  }
  if (/отпуск/.test(n)) {
    const s = findSection("leave-3-2")!;
    const s2 = findSection("leave-2-1")!;
    return {
      content: [
        "Отпуск по графику идёт без заявления. Если даты сдвигаются — заявление нужно за две недели.",
        "",
        mark(s.id, s.text),
        "",
        mark(s2.id, s2.text),
      ].join("\n"),
      meta: { sources: [s, s2].map(sourceFromSection), followUps: followUpsFor("leave") },
      draft: null,
    };
  }
  if (/служебн.*записк|срок ответа/.test(n)) {
    const s = findSection("docflow-6-2")!;
    const s2 = findSection("docflow-3-1")!;
    return {
      content: [
        "Если в самой записке срок не указан, у адресата есть пять рабочих дней на ответ.",
        "",
        mark(s.id, s.text),
        "",
        mark(s2.id, s2.text),
      ].join("\n"),
      meta: { sources: [s, s2].map(sourceFromSection), followUps: followUpsFor("memo") },
      draft: null,
    };
  }
  if (/персональн|152|пдн|персональн/.test(n)) {
    const s = findSection("pdn-2-2")!;
    const s2 = findSection("pdn-4-1")!;
    return {
      content: [
        "Персональные данные можно обрабатывать только на понятном основании — договор, закон или отдельное согласие.",
        "",
        mark(s.id, s.text),
        "",
        mark(s2.id, s2.text),
      ].join("\n"),
      meta: { sources: [s, s2].map(sourceFromSection), followUps: followUpsFor("pdn") },
      draft: null,
    };
  }
  if (/информационн(ая|ой) безопас|инцидент|парол|конфиденц/.test(n)) {
    const s = findSection("security-5-2")!;
    const s2 = findSection("security-2-4")!;
    return {
      content: [
        "В вопросах безопасности регламент требует действовать сразу и по внутренним каналам — без пересылки на личную почту.",
        "",
        mark(s2.id, s2.text),
        "",
        mark(s.id, s.text),
      ].join("\n"),
      meta: { sources: [s2, s].map(sourceFromSection), followUps: followUpsFor("security") },
      draft: null,
    };
  }
  if (/закуп|договор|поставщик|нмц/.test(n)) {
    const s = findSection("proc-2-1")!;
    const s2 = findSection("proc-3-3")!;
    return {
      content: [
        "Закупка начинается с обоснованной заявки. Договор с поставщиком нельзя подписывать в обход юридического и закупочного контура.",
        "",
        mark(s.id, s.text),
        "",
        mark(s2.id, s2.text),
      ].join("\n"),
      meta: { sources: [s, s2].map(sourceFromSection), followUps: followUpsFor("procurement") },
      draft: null,
    };
  }
  if (n.length < 8 && !/срок|шаблон|виза/.test(n)) {
    return {
      content:
        "Уточните, пожалуйста: вас интересует порядок согласования, сроки, шаблон документа или проверка уже готового файла?",
      meta: {
        clarifying: true,
        followUps: ["Порядок согласования", "Сроки", "Нужен шаблон", "Проверить документ"],
      },
      draft: input.draft,
    };
  }
  const scored = scoreSections(contextQuery);
  if (!scored.length || scored[0].score < 3) {
    return notFound();
  }
  return buildGenericAnswer(scored, raw);
}
