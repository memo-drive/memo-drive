import { describe, expect, it } from "vitest";
import type { DriveFile, PhotoMonthIndexItem } from "../../types";
import {
  buildCategoryListRequest,
  buildPhotoTimelineRequest,
  categoryFileHref,
  filterRecentVideosByMediaFilter,
  formatCategoryMonthLabel,
  isMobileCategory,
} from "./mobileCategoryActions";

describe("mobileCategoryActions", () => {
  it("builds current-category search requests without falling back to the Files page search", () => {
    expect(buildCategoryListRequest("photos", { query: "  海边  " })).toEqual({
      category: "photos",
      query: "海边",
      sort: "updated_at",
      limit: 40,
    });

    expect(buildCategoryListRequest("videos", { query: "课程", mediaFilter: "1_10m", cursor: "cursor-1" })).toEqual({
      category: "videos",
      query: "课程",
      sort: "updated_at",
      limit: 40,
      cursor: "cursor-1",
      media_filter: "1_10m",
    });
  });

  it("maps document subtypes and audio sort into the shared category query", () => {
    expect(buildCategoryListRequest("documents", { documentSubtype: "pdf" })).toMatchObject({
      category: "documents",
      document_subtype: "pdf",
    });

    expect(buildCategoryListRequest("audio", { sort: "name" })).toMatchObject({
      category: "audio",
      sort: "name",
    });
  });

  it("builds a photo timeline request for a single month and preserves search", () => {
    const month: PhotoMonthIndexItem = { year: 2026, month: 5, count: 18 };

    expect(buildPhotoTimelineRequest(month, { query: "  毕业  ", cursor: "next" })).toEqual({
      year: 2026,
      month: 5,
      query: "毕业",
      limit: 60,
      cursor: "next",
    });
  });

  it("filters recently watched videos by duration using existing media filter buckets", () => {
    const recent = [
      makeFile({ id: "short", name: "short.mp4", metadata: makeMetadata({ duration: 59.5 }) }),
      makeFile({ id: "medium", name: "medium.mp4", metadata: makeMetadata({ duration: 600 }) }),
      makeFile({ id: "long", name: "long.mp4", metadata: makeMetadata({ duration: 601 }) }),
      makeFile({ id: "unknown", name: "unknown.mp4", metadata: makeMetadata({}) }),
    ];

    expect(filterRecentVideosByMediaFilter(recent, "lt_1m").map((file) => file.id)).toEqual(["short"]);
    expect(filterRecentVideosByMediaFilter(recent, "1_10m").map((file) => file.id)).toEqual(["medium"]);
    expect(filterRecentVideosByMediaFilter(recent, "gt_10m").map((file) => file.id)).toEqual(["long"]);
    expect(filterRecentVideosByMediaFilter(recent, "all").map((file) => file.id)).toEqual([
      "short",
      "medium",
      "long",
      "unknown",
    ]);
  });

  it("links media categories to media preview while documents keep the category return target", () => {
    const photo = makeFile({ id: "photo-1", name: "旅行.jpg", path: "/Camera", mime_type: "image/jpeg" });
    const doc = makeFile({ id: "doc-1", name: "笔记.pdf", path: "/Docs", mime_type: "application/pdf" });

    expect(categoryFileHref("photos", photo)).toBe(
      "/m/media/photos/photo-1?returnTo=%2Fm%2Fcategory%2Fphotos",
    );
    expect(categoryFileHref("documents", doc)).toBe(
      "/m/preview/doc-1?path=%2FDocs&returnTo=%2Fm%2Fcategory%2Fdocuments",
    );
  });

  it("recognizes valid mobile categories and formats month labels", () => {
    expect(isMobileCategory("photos")).toBe(true);
    expect(isMobileCategory("folders")).toBe(false);
    expect(formatCategoryMonthLabel({ year: 2026, month: 5, count: 8 })).toBe("2026年5月");
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.pdf",
    path: "/",
    storage_path: "Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}

function makeMetadata(meta: Record<string, unknown>): DriveFile["metadata"] {
  return {
    file_id: "file-1",
    meta_json: JSON.stringify(meta),
    extracted_at: "2026-05-10T00:00:00Z",
  };
}
