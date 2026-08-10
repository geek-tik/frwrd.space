import { useEffect, useMemo, useState } from "react";
import { Link, Navigate, useParams } from "react-router-dom";
import { useAuth } from "../auth";
import {
  getTunnelRequest,
  listTunnelRequests,
  openTunnelRequestStream,
  type RequestDetail,
  type RequestEvent,
} from "../tunnels";

export function TunnelInspectorPage() {
  const { id } = useParams<{ id: string }>();
  const { user, loading } = useAuth();
  const [requests, setRequests] = useState<RequestEvent[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [detail, setDetail] = useState<RequestDetail | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!id || !user) return;
    listTunnelRequests(id)
      .then((data) => setRequests(data.requests))
      .catch((e) => setError(e.message));
    return openTunnelRequestStream(id, (event) => {
      setRequests((prev) => [event, ...prev.filter((r) => r.id !== event.id)].slice(0, 100));
    });
  }, [id, user]);

  useEffect(() => {
    if (!id || !selectedId) {
      setDetail(null);
      return;
    }
    getTunnelRequest(id, selectedId)
      .then(setDetail)
      .catch((e) => setError(e.message));
  }, [id, selectedId]);

  const selected = useMemo(
    () => requests.find((r) => r.id === selectedId) || null,
    [requests, selectedId],
  );

  if (loading) {
    return <main className="page">Загрузка...</main>;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  if (!id) {
    return <Navigate to="/" replace />;
  }

  return (
    <main className="page wide">
      <header className="topbar">
        <div>
          <h1>Инспектор</h1>
          <p className="subtitle">Туннель {id}</p>
        </div>
        <Link className="button-link" to="/">
          Назад
        </Link>
      </header>

      {error && <p className="error">{error}</p>}

      <div className="inspector-grid">
        <section className="card inspector-list">
          <h2>Запросы</h2>
          {requests.length === 0 ? (
            <p className="muted">Пока нет запросов через туннель</p>
          ) : (
            <ul className="request-list">
              {requests.map((req) => (
                <li key={req.id}>
                  <button
                    type="button"
                    className={selectedId === req.id ? "active" : ""}
                    onClick={() => setSelectedId(req.id)}
                  >
                    <span className={`method method-${req.method.toLowerCase()}`}>{req.method}</span>
                    <span className="path">{req.path}</span>
                    <span className={`status status-${Math.floor(req.status_code / 100)}xx`}>
                      {req.status_code}
                    </span>
                    <span className="muted">{req.duration_ms}ms</span>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </section>

        <section className="card inspector-detail">
          <h2>Детали</h2>
          {!selected && <p className="muted">Выберите запрос</p>}
          {selected && (
            <>
              <p>
                <strong>{selected.method}</strong> {selected.path}
                {selected.query ? `?${selected.query}` : ""}
              </p>
              <p className="muted">
                {selected.status_code} · {selected.duration_ms}ms ·{" "}
                {new Date(selected.created_at).toLocaleString("ru-RU")}
              </p>
            </>
          )}
          {detail && (
            <>
              <h3>Request headers</h3>
              <pre>{JSON.stringify(detail.headers, null, 2)}</pre>
              {detail.body && (
                <>
                  <h3>Request body</h3>
                  <pre>{detail.body}</pre>
                </>
              )}
              <h3>Response headers</h3>
              <pre>{JSON.stringify(detail.resp_headers, null, 2)}</pre>
              {detail.resp_body && (
                <>
                  <h3>Response body</h3>
                  <pre>{detail.resp_body}</pre>
                </>
              )}
            </>
          )}
        </section>
      </div>
    </main>
  );
}
