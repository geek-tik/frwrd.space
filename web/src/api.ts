const ACCESS_TOKEN_KEY = "forward_access_token";

export type User = {
  id: string;
  email: string;
};

export type AuthResponse = {
  access_token: string;
  expires_in: number;
  user: User;
};

export type APIToken = {
  id: string;
  name: string;
  last_used_at?: string;
  created_at: string;
};

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setAccessToken(token: string) {
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
}

export function clearAccessToken() {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
}

export async function request<T>(path: string, init: RequestInit = {}, retried = false): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has("Content-Type") && init.body) {
    headers.set("Content-Type", "application/json");
  }

  const token = getAccessToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "include",
  });

  if (
    response.status === 401 &&
    !retried &&
    !path.includes("/auth/login") &&
    !path.includes("/auth/register") &&
    !path.includes("/auth/refresh")
  ) {
    const refreshed = await refreshSession();
    if (refreshed) {
      return request(path, init, true);
    }
  }

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new ApiError(response.status, payload.error || `HTTP ${response.status}`);
  }
  return payload as T;
}

export async function refreshSession(): Promise<boolean> {
  try {
    const data = await request<{ access_token: string }>("/api/v1/auth/refresh", {
      method: "POST",
    });
    setAccessToken(data.access_token);
    return true;
  } catch {
    clearAccessToken();
    return false;
  }
}

export function register(email: string, password: string) {
  return request<AuthResponse>("/api/v1/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function login(email: string, password: string) {
  return request<AuthResponse>("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function logout() {
  return request<{ status: string }>("/api/v1/auth/logout", { method: "POST" });
}

export function getMe() {
  return request<User>("/api/v1/me");
}

export function listTokens() {
  return request<{ tokens: APIToken[] }>("/api/v1/tokens");
}

export function createToken(name: string) {
  return request<APIToken & { token: string }>("/api/v1/tokens", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function deleteToken(id: string) {
  return request<{ status: string }>(`/api/v1/tokens/${id}`, { method: "DELETE" });
}

export function getStatus() {
  return request<{ service: string; base_domain: string; version: string }>("/api/v1/status");
}
