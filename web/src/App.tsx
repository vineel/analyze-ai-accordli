import { useEffect, useState } from "react";

type Health = { ok: boolean; version: string };

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetch("/api/health")
      .then((r) => {
        if (!r.ok) throw new Error(`HTTP ${r.status}`);
        return r.json() as Promise<Health>;
      })
      .then(setHealth)
      .catch((e: Error) => setError(e.message));
  }, []);

  return (
    <main
      style={{
        fontFamily: "system-ui, sans-serif",
        padding: "2rem",
        maxWidth: "32rem",
        margin: "0 auto",
      }}
    >
      <h1>SoloMocky</h1>
      <p>
        Phase 0 skeleton. The API at <code>/api/health</code> says:
      </p>
      {error && <pre style={{ color: "crimson" }}>error: {error}</pre>}
      {!error && !health && <p>loading…</p>}
      {health && <pre>{JSON.stringify(health, null, 2)}</pre>}
    </main>
  );
}
