import { useEffect, useState } from "react";
import { api, type MatterListItem } from "./api";

type Props = {
  onOpen: (id: string) => void;
};

export default function MatterList({ onOpen }: Props) {
  const [matters, setMatters] = useState<MatterListItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [creating, setCreating] = useState(false);

  const refresh = () =>
    api.listMatters().then(setMatters).catch((e: Error) => setError(e.message));

  useEffect(() => {
    refresh();
  }, []);

  const onContinue = async () => {
    setCreating(true);
    setError(null);
    try {
      const m = await api.createMatter();
      setShowModal(false);
      onOpen(m.id);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setCreating(false);
    }
  };

  return (
    <div>
      <div className="toolbar">
        <button className="primary" onClick={() => setShowModal(true)}>
          + New Matter
        </button>
        <button onClick={refresh}>Refresh</button>
      </div>

      {error && <div className="error">{error}</div>}

      {matters === null && <p>Loading…</p>}
      {matters && matters.length === 0 && (
        <div className="empty">
          <p>No matters yet.</p>
          <p>Click <strong>+ New Matter</strong> to create one from the bundled sample.</p>
        </div>
      )}
      {matters && matters.length > 0 && (
        <ul className="matter-list">
          {matters.map((m) => (
            <li key={m.id}>
              <div>
                <div className="title">
                  <a
                    href={`#/matters/${m.id}`}
                    onClick={(e) => {
                      e.preventDefault();
                      onOpen(m.id);
                    }}
                  >
                    {m.title}
                  </a>
                </div>
                <div className="meta">
                  {m.status} · {new Date(m.created_at).toLocaleString()}
                </div>
              </div>
            </li>
          ))}
        </ul>
      )}

      {showModal && (
        <div className="modal-backdrop" onClick={() => !creating && setShowModal(false)}>
          <div className="modal" onClick={(e) => e.stopPropagation()}>
            <h2>New Matter</h2>
            <p>
              Placeholder for upload agreement. Click <strong>Continue</strong> to use the
              bundled sample agreement.
            </p>
            <div className="modal-actions">
              <button onClick={() => setShowModal(false)} disabled={creating}>
                Cancel
              </button>
              <button className="primary" onClick={onContinue} disabled={creating}>
                {creating ? "Creating…" : "Continue"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
