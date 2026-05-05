import { useEffect, useState } from "react";
import "./styles.css";
import MatterList from "./MatterList";
import MatterDetail from "./MatterDetail";

// Hash-based router. Two routes: "" (list) and "matters/<id>" (detail).
function parseHash(): { kind: "list" } | { kind: "detail"; id: string } {
  const h = window.location.hash.replace(/^#\/?/, "");
  const m = h.match(/^matters\/([0-9a-f-]+)$/i);
  if (m) return { kind: "detail", id: m[1] };
  return { kind: "list" };
}

export default function App() {
  const [route, setRoute] = useState(parseHash());

  useEffect(() => {
    const onHash = () => setRoute(parseHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  const navList = () => {
    window.location.hash = "";
  };
  const navDetail = (id: string) => {
    window.location.hash = `/matters/${id}`;
  };

  return (
    <div className="container">
      <header className="header">
        <h1>
          <a
            href="#/"
            style={{ textDecoration: "none", color: "inherit" }}
            onClick={(e) => {
              e.preventDefault();
              navList();
            }}
          >
            SoloMocky
          </a>
        </h1>
        <span className="who">mocky@accordli.local · Mocky Org</span>
      </header>
      {route.kind === "list" && <MatterList onOpen={navDetail} />}
      {route.kind === "detail" && <MatterDetail id={route.id} onBack={navList} />}
    </div>
  );
}
