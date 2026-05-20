import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  batchDeleteFiles,
  batchMoveFiles,
  listPhotoMonths,
  listRecentlyViewedFiles,
  markFileViewed,
  queryFiles,
  queryPhotoTimeline,
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

  it("wraps category, recent, timeline, view, and batch endpoints with typed payloads", async () => {
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
});
