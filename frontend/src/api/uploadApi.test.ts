import { afterEach, expect, it, vi } from "vitest";
import { initUpload, prepareDirectoryUpload } from "./uploadApi";

afterEach(() => {
  vi.unstubAllGlobals();
});

it("sends the selected conflict policy when initializing an Upload Session", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({
      id: "upload-1",
      file_name: "report.pdf",
      requested_name: "report.pdf",
      resolved_name: "report (1).pdf",
      overwrite_policy: "rename",
      file_size: 3,
      chunk_size: 64,
      uploaded_chunks: [],
      dest_path: "/Docs",
      status: "uploading",
      created_at: "2026-07-28T00:00:00Z",
      expires_at: "2026-07-29T00:00:00Z",
    }), {
      status: 201,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);

  const file = new File(["pdf"], "report.pdf", { type: "application/pdf" });
  await initUpload(file, "/Docs", "rename");

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/upload/init",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({
        file_name: "report.pdf",
        file_size: 3,
        dest_path: "/Docs",
        overwrite_policy: "rename",
      }),
    }),
  );
});

it("preserves directory relative paths when preparing an upload batch", async () => {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify({ batch_id: "batch-1", folders: [], entries: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  vi.stubGlobal("fetch", fetchMock);
  const file = new File(["hello"], "main.ts", { type: "text/plain" });

  await prepareDirectoryUpload("/Docs", [{
    clientId: "local-1",
    relativePath: "Project/src/main.ts",
    file,
  }]);

  expect(fetchMock).toHaveBeenCalledWith(
    "/api/upload/directory/prepare",
    expect.objectContaining({
      method: "POST",
      body: JSON.stringify({
        dest_path: "/Docs",
        entries: [{
          client_id: "local-1",
          relative_path: "Project/src/main.ts",
          file_size: 5,
        }],
      }),
    }),
  );
});
