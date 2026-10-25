export type AttachedFile = {
  name: string;
  type: string;
  size: number;
  url?: string;
  /** base64 payload — sent for .docx so the backend can audit it */
  data?: string;
};
export type SourceRef = {
  id: string;
  paragraphId?: string;
  documentTitle: string;
  clause: string;
  excerpt: string;
  updatedAt: string;
};
export type ProcessStep = {
  id: string;
  step?: number;
  title: string;
  owner?: string;
  timeframe?: string;
  deadline?: string;
  department?: string;
  description?: string;
  mandatory?: boolean;
};
export type GeneratedDocument = {
  id?: string;
  filename?: string;
  fileName?: string;
  title: string;
  kind?: string;
  preview?: string;
  content?: string;
  body?: string;
  format?: string;
  url?: string;
  /** server-side artifact id — download via GET /api/artifacts/{id} */
  artifactId?: string;
  moduleId?: string;
};
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
export type VisaRequirementFE = {
  department: string;
  required: boolean;
  reason?: string;
  deadline?: string;
};
export type AuditReportFE = {
  fileName?: string;
  status: "READY" | "WARNINGS" | "CRITICAL" | string;
  statusSummary?: string;
  detectedKind?: string;
  matchedModuleID?: string;
  totalFindings?: number;
  criticalCount?: number;
  warningCount?: number;
  infoCount?: number;
  findings?: AuditFinding[];
  visaRouting?: {
    requiredVisas?: VisaRequirementFE[];
    appliedRules?: string[];
    modulePrivilege?: boolean;
    privilegeNote?: string;
    normativeBasis?: string;
  };
};
export type MessageMeta = {
  intent?: string;
  sources?: SourceRef[];
  process?: {
    title?: string;
    steps: ProcessStep[];
  };
  document?: GeneratedDocument;
  documents?: GeneratedDocument[];
  notFound?: boolean;
  expert?: boolean;
  clarifying?: boolean;
  followUps?: string[];
  files?: AttachedFile[];
  wizard?: WizardMeta;
  audit?: AuditReportFE;
};
export type ConversationDraft = {
  templateId?: string | null;
  step?: number;
  fields?: Record<string, string>;
} | null;
export type Template = {
  id: string;
  title: string;
  category: string;
  description: string;
  fileUrl?: string;
};
