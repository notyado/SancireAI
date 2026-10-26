/**
 * Payload types shared with the Go backend: dialog.Meta (internal/dialog),
 * docx.AuditReport (internal/docx), matrix routing and the REST responses of
 * internal/api. Keep them in sync with the JSON tags on the Go side.
 */

export type AttachedFile = {
  name: string;
  type: string;
  size: number;
  url?: string;
  /** base64 payload — sent for .docx so the backend can audit it */
  data?: string;
};

/** A normative fragment cited in an answer (dialog.Source). */
export type SourceRef = {
  id: string;
  paragraphId?: string;
  documentTitle: string;
  clause: string;
  excerpt: string;
  updatedAt: string;
};

/** One step of the rendered «Порядок» diagram (dialog.ProcessStep). */
export type ProcessStep = {
  id: string;
  title: string;
  description?: string;
  deadline?: string;
  department?: string;
};

/** A generated modular document ready for download (dialog.DocumentCard). */
export type GeneratedDocument = {
  title: string;
  kind?: string;
  fileName?: string;
  preview?: string;
  /** server-side artifact id — download via GET /api/artifacts/{id} */
  artifactId?: string;
  moduleId?: string;
};

/** State of the interactive fill-in wizard (dialog.WizardMeta). */
export type WizardMeta = {
  moduleId: string;
  code: string;
  title: string;
  phase: "ask" | "confirm" | string;
  step: number;
  total: number;
  field?: {
    key: string;
    label: string;
    question: string;
    example?: string;
    note?: string;
    options?: string[];
    required?: boolean;
  };
  values?: Record<string, string>;
};

export type AuditFinding = {
  id?: string;
  severity: "CRITICAL" | "WARNING" | "INFO" | string;
  title: string;
  description?: string;
  location?: string;
  ruleCitation?: string;
  recommendation?: string;
};

export type VisaRequirement = {
  department: string;
  required: boolean;
  reason?: string;
  deadline?: string;
};

/** One row of the mandatory visa matrix (matrix.MatrixRow). */
export type MatrixRow = {
  id: string;
  rowNumber: number;
  category: string;
  description: string;
  legal: boolean;
  dou: boolean;
  hr: boolean;
  otherDept?: string[];
  condition?: string;
};

/** One stage of the Глава 10 route (matrix.RouteStep). */
export type RouteStep = {
  order: number;
  stage: string;
  actor: string;
  description: string;
  deadline: string;
};

/** Calculated visa route with its deterministic justification. */
export type VisaRouting = {
  kind?: string;
  matchedRows?: MatrixRow[];
  requiredVisas?: VisaRequirement[];
  routeSteps?: RouteStep[];
  modulePrivilege?: boolean;
  privilegeNote?: string;
  privilegeWarning?: string;
  appliedRules?: string[];
  normativeBasis?: string;
};

/** docx.AuditReport as returned by POST /api/documents/audit. */
export type AuditReport = {
  fileName?: string;
  fileSize?: number;
  status: "READY" | "WARNINGS" | "CRITICAL" | string;
  statusSummary?: string;
  detectedKind?: string;
  detectedModule?: string;
  matchedModuleID?: string;
  totalFindings?: number;
  criticalCount?: number;
  warningCount?: number;
  infoCount?: number;
  findings?: AuditFinding[];
  visaRouting?: VisaRouting;
};

/** Metadata attached to every assistant message (dialog.Meta). */
export type MessageMeta = {
  intent?: string;
  sources?: SourceRef[];
  process?: {
    title?: string;
    steps: ProcessStep[];
  };
  document?: GeneratedDocument;
  notFound?: boolean;
  expert?: boolean;
  clarifying?: boolean;
  followUps?: string[];
  files?: AttachedFile[];
  wizard?: WizardMeta;
  audit?: AuditReport;
};

/**
 * Wizard progress persisted server-side in Conversation.Draft (dialog.State).
 * `null` means "no wizard in progress" — the backend keeps the draft on a
 * title/pin update and clears it when the document is generated or cancelled.
 */
export type ConversationDraft = {
  moduleId: string;
  phase: "ask" | "confirm" | string;
  step: number;
  values: Record<string, string>;
} | null;

/** One variable field of a modular document (docx.ModuleField). */
export type ModuleField = {
  key: string;
  label: string;
  question?: string;
  placeholder?: string;
  type: "text" | "date" | "choice" | "textarea" | string;
  required?: boolean;
  /** routing fields feed the visa matrix and {{when …}} blocks, not a token */
  routing?: boolean;
  options?: string[];
  syncTargets?: string[];
  helperNote?: string;
};

/** One modular document of the catalogue (docx.ModuleDefinition). */
export type ModuleInfo = {
  id: string;
  code: string;
  title: string;
  category?: string;
  description?: string;
  kind?: string;
  fields: ModuleField[];
};
