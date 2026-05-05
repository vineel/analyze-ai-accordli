import { useEffect, useState } from "react";
import { api, type LensRun, type MatterDetail as Detail } from "./api";

type Props = {
  id: string;
  onBack: () => void;
};

const LENS_LABELS: Record<string, string> = {
  entities_v1: "Entities",
  open_questions_v1: "Open Questions",
};

const LENS_NOUNS: Record<string, string> = {
  entities_v1: "facts",
  open_questions_v1: "questions",
};

export default function MatterDetail({ id, onBack }: Props) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    let timer: number | undefined;

    const load = async () => {
      try {
        const d = await api.getMatter(id);
        if (cancelled) return;
        setDetail(d);
        const stillRunning =
          !d.run ||
          d.run.status === "pending" ||
          d.run.status === "running" ||
          d.lenses.some((l) => l.status === "pending" || l.status === "running");
        if (stillRunning) {
          timer = window.setTimeout(load, 2000);
        }
      } catch (e) {
        if (!cancelled) setError((e as Error).message);
      }
    };
    load();

    return () => {
      cancelled = true;
      if (timer !== undefined) window.clearTimeout(timer);
    };
  }, [id]);

  if (error) return <div className="error">{error}</div>;
  if (!detail) return <p>Loading…</p>;

  return (
    <div>
      <div className="toolbar">
        <button onClick={onBack}>← Matters</button>
      </div>
      <h2 style={{ marginTop: 0 }}>{detail.title}</h2>
      <p style={{ color: "var(--muted)", marginTop: 0 }}>
        {detail.status} · created {new Date(detail.created_at).toLocaleString()}
        {detail.run && <> · run {detail.run.status}</>}
      </p>

      {detail.summary && (
        <div className="detail-summary">
          <h3>Summary</h3>
          <div>{detail.summary}</div>
        </div>
      )}
      {!detail.summary && detail.run && detail.run.status !== "failed" && (
        <div className="detail-summary">
          <h3>Summary</h3>
          <div>
            <span className="spinner" />
            Generating…
          </div>
        </div>
      )}

      {detail.lenses.map((lr) => (
        <LensPanel key={lr.id} lr={lr} />
      ))}

      <div className="downloads">
        {detail.has_markdown && (
          <a href={api.markdownURL(detail.id)} download>
            <button>Download markdown</button>
          </a>
        )}
        <a href={api.originalURL(detail.id)} download>
          <button>Download original .docx</button>
        </a>
      </div>
    </div>
  );
}

function LensPanel({ lr }: { lr: LensRun }) {
  const [open, setOpen] = useState(false);
  const label = LENS_LABELS[lr.lens_key] ?? lr.lens_key;
  const noun = LENS_NOUNS[lr.lens_key] ?? "findings";

  let meta: React.ReactNode;
  let metaCls = "meta";
  if (lr.status === "pending" || lr.status === "running") {
    meta = (
      <>
        <span className="spinner" />
        Running…
      </>
    );
  } else if (lr.status === "completed") {
    meta = `${lr.finding_count ?? 0} ${noun}`;
    metaCls = "meta completed";
  } else if (lr.status === "failed") {
    meta = `failed${lr.error_kind ? ` (${lr.error_kind})` : ""}`;
    metaCls = "meta failed";
  }

  const expandable = lr.status === "completed" && lr.findings.length > 0;

  return (
    <section className={`lens-panel${open && expandable ? " open" : ""}`}>
      <header
        onClick={() => expandable && setOpen(!open)}
        style={{ cursor: expandable ? "pointer" : "default" }}
      >
        <span className="key">
          {label}
          {expandable && <span style={{ color: "var(--muted)" }}> {open ? "▾" : "▸"}</span>}
        </span>
        <span className={metaCls}>{meta}</span>
      </header>
      {open && expandable && (
        <div className="findings">
          {lr.findings.map((f) => (
            <div className="finding" key={f.id}>
              <div>
                {f.category && <span className="cat">{f.category}</span>}
                {f.location_hint && <span className="loc">{f.location_hint}</span>}
              </div>
              {f.excerpt && <p className="excerpt">"{f.excerpt}"</p>}
              {f.details && Object.keys(f.details).length > 0 && (
                <details>
                  <summary>Details</summary>
                  <pre>{JSON.stringify(f.details, null, 2)}</pre>
                </details>
              )}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
