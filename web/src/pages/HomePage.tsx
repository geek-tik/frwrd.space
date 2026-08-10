import { useEffect, useState } from "react";
import { Link, Navigate } from "react-router-dom";
import { getStatus } from "../api";
import { useAuth } from "../auth";
import { listTunnels, type Tunnel } from "../tunnels";

type Status = {
  service: string;
  base_domain: string;
  version: string;
};

export function HomePage() {
  const { user, loading, logout } = useAuth();
  const [status, setStatus] = useState<Status | null>(null);
  const [tunnels, setTunnels] = useState<Tunnel[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getStatus()
      .then(setStatus)
      .catch((e) => setError(e.message));
  }, []);

  useEffect(() => {
    if (!user) return;
    const load = () =>
      listTunnels()
        .then((data) => setTunnels(data.tunnels))
        .catch((e) => setError(e.message));
    load();
    const timer = setInterval(load, 5000);
    return () => clearInterval(timer);
  }, [user]);

  if (loading) {
    return <main className="page">Загрузка...</main>;
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  return (
    <main className="page">
      <header className="topbar">
        <div>
          <h1>Forward</h1>
          <p className="subtitle">{user.email}</p>
        </div>
        <div className="actions">
          <Link className="button-link" to="/tokens">
            API tokens
          </Link>
          <button type="button" onClick={() => logout()}>
            Выйти
          </button>
        </div>
      </header>

      {error && <p className="error">API недоступен: {error}</p>}

      {status && (
        <section className="card">
          <dl>
            <dt>Сервис</dt>
            <dd>{status.service}</dd>
            <dt>Домен</dt>
            <dd>{status.base_domain}</dd>
            <dt>Версия</dt>
            <dd>{status.version}</dd>
          </dl>
        </section>
      )}

      <section className="card">
        <h2>Активные туннели</h2>
        {tunnels.length === 0 ? (
          <p className="muted">Нет активных туннелей</p>
        ) : (
          <ul className="token-list">
            {tunnels.map((tunnel) => (
              <li key={tunnel.id}>
                <div>
                  <strong>{tunnel.public_url}</strong>
                  <span className="muted">:{tunnel.local_port}</span>
                </div>
                <Link className="button-link" to={`/tunnels/${tunnel.id}`}>
                  Инспектор
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  );
}
