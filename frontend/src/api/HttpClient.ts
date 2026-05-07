const TOKEN_KEY = "memodrive.token";

export interface SSEHandlers {
  onDelta?: (delta: string) => void;
  onSources?: (sources: unknown[]) => void;
  onConversation?: (id: string) => void;
  onError?: (error: string) => void;
  onDone?: () => void;
}

function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? "";
}

function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

function handleUnauth(): void {
  clearToken();
  window.location.href = "/login";
}

export class HttpError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "HttpError";
    this.status = status;
  }
}

class HttpClient {
  private baseUrl: string;

  constructor(baseUrl: string) {
    this.baseUrl = baseUrl;
  }

  private url(path: string): string {
    return `${this.baseUrl}${path}`;
  }

  assetUrl(path: string): string {
    const token = getToken();
    const sep = path.includes("?") ? "&" : "?";
    return `${this.url(path)}${token ? `${sep}token=${encodeURIComponent(token)}` : ""}`;
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    const token = getToken();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    // Auto-set JSON content-type for requests with a JSON body
    if (
      init.body &&
      !(init.body instanceof Blob) &&
      !headers.has("Content-Type")
    ) {
      headers.set("Content-Type", "application/json");
    }

    const response = await fetch(this.url(path), { ...init, headers });

    if (response.status === 401) {
      handleUnauth();
      throw new HttpError(response.status, "Unauthorized");
    }

    if (!response.ok) {
      const message = await response.text().catch(() => response.statusText);
      throw new HttpError(response.status, message || response.statusText);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    const contentType = response.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/json")) {
      return (await response.json()) as T;
    }
    return (await response.text()) as T;
  }

  async get<T>(path: string): Promise<T> {
    return this.request<T>(path);
  }

  async post<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: "POST",
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  }

  async patch<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: "PATCH",
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  }

  async put<T>(path: string, body?: unknown): Promise<T> {
    return this.request<T>(path, {
      method: "PUT",
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>(path, { method: "DELETE" });
  }

  async patchBlob<T>(
    path: string,
    blob: Blob,
    signal?: AbortSignal,
  ): Promise<T> {
    return this.request<T>(path, {
      method: "PATCH",
      body: blob,
      signal,
    });
  }

  async streamSSE(
    path: string,
    body: unknown,
    handlers: SSEHandlers,
    signal?: AbortSignal,
  ): Promise<void> {
    const headers = new Headers({
      "Content-Type": "application/json",
    });
    const token = getToken();
    if (token) {
      headers.set("Authorization", `Bearer ${token}`);
    }

    const response = await fetch(this.url(path), {
      method: "POST",
      headers,
      body: JSON.stringify(body),
      signal,
    });

    if (response.status === 401) {
      handleUnauth();
      throw new HttpError(response.status, "Unauthorized");
    }

    if (!response.ok || !response.body) {
      throw new HttpError(
        response.status,
        await response.text().catch(() => response.statusText),
      );
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const events = buffer.split("\n\n");
        buffer = events.pop() ?? "";
        for (const event of events) {
          dispatchSSEEvent(event, handlers);
        }
      }
      if (buffer.trim()) {
        dispatchSSEEvent(buffer, handlers);
      }
    } catch (err) {
      if (signal?.aborted) return;
      handlers.onError?.(err instanceof Error ? err.message : "SSE stream interrupted");
      throw err;
    } finally {
      reader.releaseLock();
    }
  }

  async postSSE(
    path: string,
    body: unknown,
    onDelta: (delta: string) => void,
  ): Promise<void> {
    return this.streamSSE(path, body, { onDelta });
  }

  async login(password: string): Promise<{ token: string }> {
    const result = await this.post<{ token: string }>("/auth/login", {
      password,
    });
    setToken(result.token);
    return result;
  }

  async checkAuth(): Promise<{ required: boolean }> {
    return this.get<{ required: boolean }>("/auth/status");
  }
}

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? "/api";

export const httpClient = new HttpClient(API_BASE);
export { getToken, setToken, clearToken };

function dispatchSSEEvent(rawEvent: string, handlers: SSEHandlers): void {
  const lines = rawEvent.split(/\r?\n/);
  let eventName = "message";
  const dataLines: string[] = [];
  for (const line of lines) {
    if (line.startsWith("event:")) {
      eventName = line.slice(6).trim() || "message";
      continue;
    }
    if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).trimStart());
    }
  }
  if (dataLines.length === 0) return;
  const rawData = dataLines.join("\n");
  if (rawData === "{}" && eventName === "done") {
    handlers.onDone?.();
    return;
  }
  try {
    const payload = JSON.parse(rawData) as {
      delta?: string;
      error?: string;
      id?: string;
      sources?: unknown[];
    };
    switch (eventName) {
      case "conversation":
        if (typeof payload.id === "string") {
          handlers.onConversation?.(payload.id);
        }
        break;
      case "sources":
        handlers.onSources?.(Array.isArray(payload.sources) ? payload.sources : []);
        break;
      case "error":
        handlers.onError?.(payload.error || "SSE event error");
        break;
      case "done":
        handlers.onDone?.();
        break;
      default:
        if (typeof payload.delta === "string") {
          handlers.onDelta?.(payload.delta);
        }
        break;
    }
  } catch (err) {
    handlers.onError?.(err instanceof Error ? err.message : "SSE data parse failed");
  }
}
