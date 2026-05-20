import type { DriveFile, FileQueryRequest } from "../../types";
import type { MobileCategoryKey } from "./mobileCategoryActions";

export type MobileMediaCategory = Extract<MobileCategoryKey, "photos" | "videos" | "audio">;

export const MOBILE_MEDIA_CATEGORIES: MobileMediaCategory[] = ["photos", "videos", "audio"];
const DEFAULT_MEDIA_SWIPE_MIN_DISTANCE = 56;
const DEFAULT_MEDIA_SWIPE_AXIS_RATIO = 1.2;

export type MediaSwipeNavigation = "previous" | "next";

export interface MediaSwipePoint {
  x: number;
  y: number;
}

export function isMobileMediaCategory(value: string | undefined): value is MobileMediaCategory {
  return Boolean(value && MOBILE_MEDIA_CATEGORIES.includes(value as MobileMediaCategory));
}

export function buildMediaQueueRequest(
  category: MobileMediaCategory,
  options: { cursor?: string; limit?: number; query?: string } = {},
): FileQueryRequest {
  const request: FileQueryRequest = {
    category,
    sort: "updated_at",
    limit: options.limit ?? 60,
  };
  if (options.cursor) request.cursor = options.cursor;
  return request;
}

export function appendMediaQueuePage(
  currentQueue: DriveFile[],
  nextPage: DriveFile[],
  currentFile?: DriveFile,
): DriveFile[] {
  const seen = new Set<string>();
  const merged: DriveFile[] = [];
  const add = (file: DriveFile | undefined) => {
    if (!file || seen.has(file.id)) return;
    seen.add(file.id);
    merged.push(file);
  };

  if (currentFile && !currentQueue.some((file) => file.id === currentFile.id) && !nextPage.some((file) => file.id === currentFile.id)) {
    add(currentFile);
  }
  currentQueue.forEach(add);
  nextPage.forEach(add);
  return merged;
}

export function previousMediaTarget(queue: DriveFile[], currentFileId: string | undefined) {
  const index = findMediaIndex(queue, currentFileId);
  return index > 0 ? queue[index - 1] : undefined;
}

export function nextMediaTarget(queue: DriveFile[], currentFileId: string | undefined) {
  const index = findMediaIndex(queue, currentFileId);
  return index >= 0 && index < queue.length - 1 ? queue[index + 1] : undefined;
}

export function mediaReturnHref(category: MobileMediaCategory, search: string): string {
  const params = new URLSearchParams(search);
  const returnTo = params.get("returnTo");
  if (returnTo && (returnTo === "/m" || returnTo.startsWith("/m/")) && !returnTo.startsWith("//")) {
    return returnTo;
  }
  return `/m/category/${category}`;
}

export function mediaHref(
  category: MobileMediaCategory,
  fileId: string,
  returnHref: string,
): string {
  return `/m/media/${category}/${encodeURIComponent(fileId)}?returnTo=${encodeURIComponent(returnHref)}`;
}

export function mediaDeleteFallback(
  queue: DriveFile[],
  currentFileId: string,
  category: MobileMediaCategory,
  returnHref: string,
): { nextFileId?: string; href: string } {
  const index = findMediaIndex(queue, currentFileId);
  const next = index >= 0 ? queue[index + 1] ?? queue[index - 1] : undefined;
  if (!next) return { href: returnHref };
  return {
    nextFileId: next.id,
    href: mediaHref(category, next.id, returnHref),
  };
}

export function mediaSwipeNavigation(
  start: MediaSwipePoint,
  end: MediaSwipePoint,
  options: { minDistance?: number; axisRatio?: number } = {},
): MediaSwipeNavigation | null {
  const minDistance = options.minDistance ?? DEFAULT_MEDIA_SWIPE_MIN_DISTANCE;
  const axisRatio = options.axisRatio ?? DEFAULT_MEDIA_SWIPE_AXIS_RATIO;
  const deltaX = end.x - start.x;
  const deltaY = end.y - start.y;
  const horizontalDistance = Math.abs(deltaX);
  const verticalDistance = Math.abs(deltaY);

  if (horizontalDistance < minDistance) return null;
  if (horizontalDistance < verticalDistance * axisRatio) return null;

  return deltaX < 0 ? "next" : "previous";
}

function findMediaIndex(queue: DriveFile[], currentFileId: string | undefined) {
  if (!currentFileId) return -1;
  return queue.findIndex((file) => file.id === currentFileId);
}
