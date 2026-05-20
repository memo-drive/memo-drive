import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import {
  appendMediaQueuePage,
  buildMediaQueueRequest,
  mediaDeleteFallback,
  mediaReturnHref,
  mediaSwipeNavigation,
  nextMediaTarget,
  previousMediaTarget,
} from "./mobileMediaPreviewActions";

describe("mobileMediaPreviewActions", () => {
  it("loads media queues by category without carrying category-page search filters", () => {
    expect(buildMediaQueueRequest("photos", { cursor: "next", limit: 24 })).toEqual({
      category: "photos",
      sort: "updated_at",
      limit: 24,
      cursor: "next",
    });

    expect(buildMediaQueueRequest("audio", { query: "should be ignored" })).toEqual({
      category: "audio",
      sort: "updated_at",
      limit: 60,
    });
  });

  it("keeps the current file in the queue and resolves adjacent media targets", () => {
    const current = makeFile({ id: "current", name: "当前.jpg" });
    const page = [
      makeFile({ id: "previous", name: "上一张.jpg" }),
      current,
      makeFile({ id: "next", name: "下一张.jpg" }),
    ];

    const queue = appendMediaQueuePage([], page, current);

    expect(queue.map((file) => file.id)).toEqual(["previous", "current", "next"]);
    expect(previousMediaTarget(queue, "current")?.id).toBe("previous");
    expect(nextMediaTarget(queue, "current")?.id).toBe("next");
  });

  it("falls back to the current file when a paged queue has not reached it yet", () => {
    const current = makeFile({ id: "current", name: "当前.mp4", mime_type: "video/mp4" });

    const queue = appendMediaQueuePage(
      [],
      [makeFile({ id: "other", name: "其他.mp4", mime_type: "video/mp4" })],
      current,
    );

    expect(queue.map((file) => file.id)).toEqual(["current", "other"]);
  });

  it("returns to the source media category and picks a neighboring item after delete", () => {
    const queue = [
      makeFile({ id: "one", name: "1.jpg" }),
      makeFile({ id: "two", name: "2.jpg" }),
      makeFile({ id: "three", name: "3.jpg" }),
    ];

    expect(mediaReturnHref("photos", "?path=%2FCamera")).toBe("/m/category/photos");
    expect(mediaReturnHref("photos", "?returnTo=%2Fm")).toBe("/m");
    expect(mediaReturnHref("videos", "?returnTo=%2Fm%2Fcategory%2Fvideos%3Fmode%3Drecent")).toBe(
      "/m/category/videos?mode=recent",
    );
    expect(mediaDeleteFallback(queue, "two", "photos", "/m/category/photos")).toEqual({
      nextFileId: "three",
      href: "/m/media/photos/three?returnTo=%2Fm%2Fcategory%2Fphotos",
    });
    expect(mediaDeleteFallback([queue[0]], "one", "photos", "/m/category/photos")).toEqual({
      href: "/m/category/photos",
    });
  });

  it("maps deliberate horizontal swipes to neighboring media navigation", () => {
    expect(mediaSwipeNavigation({ x: 160, y: 240 }, { x: 72, y: 250 })).toBe("next");
    expect(mediaSwipeNavigation({ x: 72, y: 240 }, { x: 160, y: 250 })).toBe("previous");
    expect(mediaSwipeNavigation({ x: 160, y: 240 }, { x: 132, y: 244 })).toBeNull();
    expect(mediaSwipeNavigation({ x: 160, y: 240 }, { x: 96, y: 330 })).toBeNull();
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.jpg",
    path: "/",
    storage_path: "Memo.jpg",
    size: 1024,
    mime_type: "image/jpeg",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
