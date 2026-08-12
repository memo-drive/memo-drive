import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  batchDeleteFiles,
  batchMoveFiles,
	copyFile,
	deleteFileVersion,
	folderZIPDownloadUrl,
	fileVersionDownloadUrl,
	getDownloadText,
	listFileVersions,
  listFiles,
  listPhotoMonths,
  listRecentlyViewedFiles,
  markFileViewed,
  preflightFileConflicts,
  queryFiles,
	queryPhotoTimeline,
	restoreFileVersion,
} from "./fileApi";

describe("mobile home media file API", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    fetchMock.mockImplementation(() => Promise.resolve(
      new Response(JSON.stringify({ items: [], next_cursor: "", has_more: false }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    ));
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    fetchMock.mockReset();
  });

  it("requests a specific Folder cursor page", async () => {
    await listFiles("/Docs", { sort: "name", cursor: "next page", limit: 25 });

    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files?path=%2FDocs&sort=name&cursor=next%20page&limit=25",
      expect.any(Object),
    );
  });

  it("wraps category, recent, timeline, view, and batch endpoints with typed payloads", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ files: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await listFiles("/Docs");
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files?path=%2FDocs&sort=created_at",
      expect.any(Object),
    );

    await queryFiles({ category: "photos", query: "trip", limit: 30 });
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/query",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ category: "photos", query: "trip", limit: 30 }),
      }),
    );

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ files: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await listRecentlyViewedFiles(8);
    expect(fetchMock).toHaveBeenLastCalledWith("/api/files/recent?limit=8", expect.any(Object));

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ months: [{ year: 2026, month: 5, count: 12 }] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await listPhotoMonths();
    expect(fetchMock).toHaveBeenLastCalledWith("/api/files/photos/months", expect.any(Object));

    await queryPhotoTimeline({ year: 2026, month: 5, query: "trip", limit: 60 });
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/photos/timeline",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ year: 2026, month: 5, query: "trip", limit: 60 }),
      }),
    );

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ id: "file-1" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await markFileViewed("file-1");
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/file-1/view",
      expect.objectContaining({ method: "POST" }),
    );

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ total: 2, succeeded: 1, failed: 1 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await batchMoveFiles(["a", "b"], "/Target");
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/batch/move",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ file_ids: ["a", "b"], path: "/Target" }),
      }),
    );

    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ total: 2, succeeded: 2, failed: 0 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
    await batchDeleteFiles(["a", "b"]);
    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/batch/delete",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ file_ids: ["a", "b"] }),
      }),
    );
  });

  it("preflights a batch of File names in one request", async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({
        items: [{
          requested_name: "report.pdf",
          normalized_name: "report.pdf",
          conflict: true,
          existing_file_id: "file-1",
          rename_suggestion: "report (1).pdf",
        }],
      }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const result = await preflightFileConflicts("/Docs", ["report.pdf"]);

    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/conflicts",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({ path: "/Docs", names: ["report.pdf"] }),
      }),
    );
    expect(result.items[0]?.rename_suggestion).toBe("report (1).pdf");
  });

  it("uses the browser session cookie for fetched file content", async () => {
    fetchMock.mockResolvedValueOnce(new Response("hello", { status: 200 }));

    await getDownloadText("file-1");

    expect(fetchMock).toHaveBeenLastCalledWith(
      "/api/files/file-1/download",
      expect.objectContaining({ credentials: "include" }),
    );
  });

	it("copies a File tree with the shared conflict policy contract", async () => {
		fetchMock.mockResolvedValueOnce(
			new Response(JSON.stringify({
				file: { id: "copy-1", name: "Report copy", is_dir: true },
				summary: { files: 2, folders: 1 },
			}), {
				status: 201,
				headers: { "Content-Type": "application/json" },
			}),
		);

		await copyFile("folder-1", {
			path: "/Target",
			name: "Report copy",
			conflictPolicy: "rename",
		});

		expect(fetchMock).toHaveBeenLastCalledWith(
			"/api/files/folder-1/copy",
			expect.objectContaining({
				method: "POST",
				body: JSON.stringify({
					path: "/Target",
					name: "Report copy",
					conflict_policy: "rename",
				}),
			}),
		);
	});

	it("builds the Folder ZIP download URL", () => {
		expect(folderZIPDownloadUrl("folder 1")).toBe("/api/files/folder%201/download?archive=zip");
	});

	it("lists the historical versions of one File", async () => {
		fetchMock.mockResolvedValueOnce(
			new Response(JSON.stringify({ versions: [{ id: "version-1", version_no: 1 }] }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		const result = await listFileVersions("file 1");
		expect(fetchMock).toHaveBeenLastCalledWith(
			"/api/files/file%201/versions",
			expect.any(Object),
		);
		expect(result.versions[0]?.version_no).toBe(1);
	});

	it("restores one historical File Version", async () => {
		fetchMock.mockResolvedValueOnce(
			new Response(JSON.stringify({ file: { id: "file-1" }, task_id: "task-1" }), {
				status: 200,
				headers: { "Content-Type": "application/json" },
			}),
		);
		const result = await restoreFileVersion("file 1", "version 1");
		expect(fetchMock).toHaveBeenLastCalledWith(
			"/api/files/file%201/versions/version%201/restore",
			expect.objectContaining({ method: "POST" }),
		);
		expect(result.task_id).toBe("task-1");
	});

	it("deletes one historical File Version", async () => {
		fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));
		await deleteFileVersion("file 1", "version 1");
		expect(fetchMock).toHaveBeenLastCalledWith(
			"/api/files/file%201/versions/version%201",
			expect.objectContaining({ method: "DELETE" }),
		);
	});

	it("builds a credential-free File Version download URL", () => {
		expect(fileVersionDownloadUrl("file 1", "version 1")).toBe(
			"/api/files/file%201/versions/version%201/download",
		);
	});
});
