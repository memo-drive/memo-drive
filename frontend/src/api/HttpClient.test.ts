import { afterEach, expect, it, vi } from "vitest";
import { HttpError, httpClient } from "./HttpClient";

afterEach(() => {
	vi.unstubAllGlobals();
});

it("never places the main JWT in an asset URL", () => {
  vi.stubGlobal("localStorage", {
    getItem: vi.fn().mockReturnValue("main.jwt.token"),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  });

  expect(httpClient.assetUrl("/files/file-1/thumbnail")).toBe(
    "/api/files/file-1/thumbnail",
  );
});

it("includes the browser session cookie with API requests", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);

  await httpClient.get("/auth/status");

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/auth/status",
    expect.objectContaining({ credentials: "include" }),
  );
});

it("adds CSRF proof to cookie-authenticated writes", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);

  await httpClient.post("/folders", { path: "/", name: "Docs" });

  const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
  expect(new Headers(init.headers).get("X-MemoDrive-CSRF")).toBe("1");
});

it("does not persist the bearer token after browser login", async () => {
  const setItem = vi.fn();
  vi.stubGlobal("localStorage", {
    getItem: vi.fn().mockReturnValue(null),
    setItem,
    removeItem: vi.fn(),
  });
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(
    JSON.stringify({ token: "bearer.jwt.token" }),
    { status: 200, headers: { "Content-Type": "application/json" } },
  )));

  await httpClient.login("correct-password");

  expect(setItem).not.toHaveBeenCalled();
});

it("revokes the server session before clearing browser auth state", async () => {
  const removeItem = vi.fn();
  vi.stubGlobal("localStorage", {
    getItem: vi.fn().mockReturnValue(null),
    setItem: vi.fn(),
    removeItem,
  });
  const fetchMock = vi.fn().mockResolvedValue(new Response(null, { status: 204 }));
  vi.stubGlobal("fetch", fetchMock);

  await httpClient.logout("all");

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/auth/logout",
    expect.objectContaining({ method: "POST", credentials: "include" }),
  );
  const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
  expect(init.body).toBe(JSON.stringify({ scope: "all" }));
  expect(removeItem).toHaveBeenCalledWith("memodrive.token");
});

it("sends cookie and CSRF proof with SSE writes", async () => {
  const fetchMock = vi.fn().mockResolvedValue(new Response("event: done\ndata: {}\n\n", {
    status: 200,
    headers: { "Content-Type": "text/event-stream" },
  }));
  vi.stubGlobal("fetch", fetchMock);

  await httpClient.streamSSE("/ai/chat", { prompt: "hello" }, {});

  const init = fetchMock.mock.calls[0]?.[1] as RequestInit;
  expect(init.credentials).toBe("include");
  expect(new Headers(init.headers).get("X-MemoDrive-CSRF")).toBe("1");
});

it("sends cookie and CSRF proof with progress uploads", async () => {
  let created: FakeXMLHttpRequest | undefined;
  class FakeXMLHttpRequest {
    withCredentials = false;
    status = 204;
    statusText = "No Content";
    responseText = "";
    requestHeaders = new Map<string, string>();
    upload = { addEventListener: vi.fn() };
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    onabort: (() => void) | null = null;

    constructor() {
      created = this;
    }

    open() {}
    abort() {}
    getResponseHeader() { return null; }
    setRequestHeader(name: string, value: string) { this.requestHeaders.set(name, value); }
    send() { this.onload?.(); }
  }
  vi.stubGlobal("XMLHttpRequest", FakeXMLHttpRequest);

  await httpClient.patchBlob(
    "/upload/session-1",
    new Blob(["chunk"]),
    undefined,
    vi.fn(),
  );

  expect(created?.withCredentials).toBe(true);
  expect(created?.requestHeaders.get("X-MemoDrive-CSRF")).toBe("1");
});

it("preserves structured API error fields", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(
    new Response(JSON.stringify({
      error: {
        code: "path_conflict",
        message: "target already exists",
        retryable: false,
        details: {
          path: "/Docs",
          name: "report.pdf",
          existing_file_id: "file-1",
        },
      },
    }), {
      status: 409,
      headers: { "Content-Type": "application/json" },
    }),
  ));

  try {
    await httpClient.post("/upload/init", {});
    throw new Error("expected request to fail");
  } catch (error) {
    expect(error).toBeInstanceOf(HttpError);
    expect(error).toMatchObject({
      status: 409,
      code: "path_conflict",
      message: "target already exists",
      retryable: false,
      details: {
        path: "/Docs",
        name: "report.pdf",
        existing_file_id: "file-1",
      },
    });
  }
});
