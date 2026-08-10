import { ApiError, getAccessToken, request } from "./api";

export type Tunnel = {
  id: string;
  subdomain: string;
  public_url: string;
  local_port: number;
  protocol: string;
  status: string;
};

export type RequestEvent = {
  id: string;
  tunnel_id: string;
  method: string;
  path: string;
  query?: string;
  status_code: number;
  duration_ms: number;
  created_at: string;
};

export type RequestDetail = RequestEvent & {
  headers: Record<string, string>;
  body?: string;
  resp_headers: Record<string, string>;
  resp_body?: string;
};

export function listTunnels() {
  return request<{ tunnels: Tunnel[] }>("/api/v1/tunnels");
}

export function listTunnelRequests(tunnelId: string) {
  return request<{ requests: RequestEvent[] }>(`/api/v1/tunnels/${tunnelId}/requests`);
}

export function getTunnelRequest(tunnelId: string, requestId: string) {
  return request<RequestDetail>(`/api/v1/tunnels/${tunnelId}/requests/${requestId}`);
}

export function openTunnelRequestStream(
  tunnelId: string,
  onEvent: (event: RequestEvent) => void,
  onError?: (error: Error) => void,
): () => void {
  const token = getAccessToken();
  const controller = new AbortController();

  (async () => {
    try {
      const response = await fetch(`/api/v1/tunnels/${tunnelId}/requests/stream`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        credentials: "include",
        signal: controller.signal,
      });
      if (!response.ok || !response.body) {
        throw new ApiError(response.status, `HTTP ${response.status}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const parts = buffer.split("\n\n");
        buffer = parts.pop() || "";
        for (const part of parts) {
          const dataLine = part.split("\n").find((l) => l.startsWith("data: "));
          if (!dataLine) continue;
          const payload = JSON.parse(dataLine.slice(6)) as RequestEvent;
          onEvent(payload);
        }
      }
    } catch (err) {
      if (!controller.signal.aborted && onError && err instanceof Error) {
        onError(err);
      }
    }
  })();

  return () => controller.abort();
}
