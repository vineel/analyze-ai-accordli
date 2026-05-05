// Tiny hand-rolled API client. No fetch wrapper, no react-query — the
// surface is small enough to do useEffect + setState directly.

export type MatterListItem = {
  id: string;
  title: string;
  status: "draft" | "locked";
  created_at: string;
  locked_at?: string;
  has_original?: boolean;
  has_markdown?: boolean;
};

export type Finding = {
  id: string;
  category?: string;
  excerpt?: string;
  location_hint?: string;
  details: Record<string, unknown>;
};

export type LensRun = {
  id: string;
  lens_key: string;
  status: "pending" | "running" | "completed" | "failed";
  finding_count?: number;
  error_kind?: string;
  findings: Finding[];
};

export type Run = {
  id: string;
  status: "pending" | "running" | "completed" | "partial" | "failed";
  vendor?: string;
  created_at: string;
  completed_at?: string;
};

export type MatterDetail = MatterListItem & {
  run?: Run;
  lenses: LensRun[];
  summary?: string;
};

async function jsonFetch<T>(input: RequestInfo, init?: RequestInit): Promise<T> {
  const r = await fetch(input, init);
  if (!r.ok) {
    let body = "";
    try {
      body = await r.text();
    } catch {
      // ignore
    }
    throw new Error(`HTTP ${r.status}: ${body}`);
  }
  return (await r.json()) as T;
}

export const api = {
  listMatters: () => jsonFetch<MatterListItem[]>("/api/matters"),

  createMatter: (title?: string) =>
    jsonFetch<MatterListItem>("/api/matters", {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: JSON.stringify({ title: title ?? "" }),
    }),

  getMatter: (id: string) => jsonFetch<MatterDetail>(`/api/matters/${id}`),

  markdownURL: (id: string) => `/api/matters/${id}/document/markdown`,
  originalURL: (id: string) => `/api/matters/${id}/document/original`,
};
