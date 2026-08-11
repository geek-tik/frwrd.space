import { FormEvent, useEffect, useState } from "react";
import { Navigate } from "react-router-dom";
import { createToken, deleteToken, listTokens, type APIToken } from "../api";
import { useAuth } from "../auth";

export function TokensPage() {
  const { user, loading } = useAuth();
  const [tokens, setTokens] = useState<APIToken[]>([]);
  const [name, setName] = useState("default");
  const [createdToken, setCreatedToken] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  const load = async () => {
    const data = await listTokens();
    setTokens(data.tokens);
  };

  useEffect(() => {
    if (user) {
      load().catch((e) => setError(e.message));
    }
  }, [user]);

  if (loading) {
    return <main className="page">Загрузка...</main>;
  }

  if (!user) {
    return <Navigate to="/login" replace />;
  }

  const onCreate = async (e: FormEvent) => {
    e.preventDefault();
    setPending(true);
    setError(null);
    setCreatedToken(null);
    try {
      const token = await createToken(name);
      setCreatedToken(token.token);
      setName("default");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось создать токен");
    } finally {
      setPending(false);
    }
  };

  const onDelete = async (id: string) => {
    setError(null);
    try {
      await deleteToken(id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Не удалось удалить токен");
    }
  };

  const copy = async (value: string) => {
    await navigator.clipboard.writeText(value);
  };

  return (
    <main className="page">
      <h1>API tokens</h1>
      <p className="subtitle">Используйте токен в контейнере агента через FORWARD_API_TOKEN</p>

      <form className="card form inline" onSubmit={onCreate}>
        <label>
          <span>Название</span>
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <button type="submit" disabled={pending}>
          {pending ? "Создаём..." : "Создать токен"}
        </button>
      </form>

      {createdToken && (
        <section className="card success">
          <p>Сохраните токен — он больше не будет показан:</p>
          <code className="token">{createdToken}</code>
          <button type="button" onClick={() => copy(createdToken)}>
            Скопировать
          </button>
        </section>
      )}

      {error && <p className="error">{error}</p>}

      <section className="card">
        <h2>Ваши токены</h2>
        {tokens.length === 0 ? (
          <p className="muted">Токенов пока нет</p>
        ) : (
          <ul className="token-list">
            {tokens.map((token) => (
              <li key={token.id}>
                <div>
                  <strong>{token.name}</strong>
                  <span className="muted">
                    {new Date(token.created_at).toLocaleString("ru-RU")}
                  </span>
                </div>
                <button type="button" className="danger" onClick={() => onDelete(token.id)}>
                  Удалить
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="card muted">
        <h2>Запуск агента</h2>
        <pre>{String.raw`# локальный сервис уже слушает :3000
FORWARD_API_TOKEN=fwd_... \
FORWARD_SERVER_URL=wss://connect.frwrd.space/agent/connect \
FORWARD_DASHBOARD_URL=https://frwrd.space \
docker compose --profile tunnel run --no-deps --rm agent http 3000`}</pre>
      </section>

      <section className="card muted">
        <h2>Проверка токена</h2>
        <p className="muted">Без туннеля — только проверить, что FORWARD_API_TOKEN принимается сервером.</p>
        <pre>{String.raw`FORWARD_API_TOKEN=fwd_... \
FORWARD_SERVER_URL=wss://connect.frwrd.space/agent/connect \
docker compose --profile tunnel run --no-deps --rm agent auth verify`}</pre>
      </section>
    </main>
  );
}
