import type {
  DriveFile,
  FileQueryCategory,
  FileQueryDocumentSubtype,
  FileQueryMediaFilter,
  FileQueryRequest,
  PhotoMonthIndexItem,
  PhotoTimelineRequest,
} from "../../types";
import { mobilePreviewHref, normalizeMobilePath } from "../utils/mobilePath";

export type MobileCategoryKey = Exclude<FileQueryCategory, "all">;

export const MOBILE_CATEGORY_KEYS: MobileCategoryKey[] = [
  "photos",
  "videos",
  "documents",
  "audio",
];

export interface BuildCategoryListOptions {
  query?: string;
  sort?: string;
  cursor?: string;
  limit?: number;
  mediaFilter?: FileQueryMediaFilter | string;
  documentSubtype?: FileQueryDocumentSubtype | string;
}

export function isMobileCategory(value: string | undefined): value is MobileCategoryKey {
  return Boolean(value && MOBILE_CATEGORY_KEYS.includes(value as MobileCategoryKey));
}

export function buildCategoryListRequest(
  category: MobileCategoryKey,
  options: BuildCategoryListOptions = {},
): FileQueryRequest {
  const request: FileQueryRequest = {
    category,
    sort: options.sort ?? "updated_at",
    limit: options.limit ?? 40,
  };
  const query = options.query?.trim();
  if (query) request.query = query;
  if (options.cursor) request.cursor = options.cursor;
  if (category === "videos" && options.mediaFilter && options.mediaFilter !== "all") {
    request.media_filter = options.mediaFilter;
  }
  if (category === "documents" && options.documentSubtype && options.documentSubtype !== "all") {
    request.document_subtype = options.documentSubtype;
  }
  return request;
}

export function buildPhotoTimelineRequest(
  month: PhotoMonthIndexItem,
  options: { query?: string; cursor?: string; limit?: number } = {},
): PhotoTimelineRequest {
  const request: PhotoTimelineRequest = {
    year: month.year,
    month: month.month,
    limit: options.limit ?? 60,
  };
  const query = options.query?.trim();
  if (query) request.query = query;
  if (options.cursor) request.cursor = options.cursor;
  return request;
}

export function categoryFileHref(category: MobileCategoryKey, file: DriveFile): string {
  if (category === "photos" || category === "videos" || category === "audio") {
    return `/m/media/${category}/${encodeURIComponent(file.id)}?returnTo=${encodeURIComponent(`/m/category/${category}`)}`;
  }
  const path = normalizeMobilePath(file.path || "/");
  return mobilePreviewHref(file.id, path, `/m/category/${category}`);
}

export function formatCategoryMonthLabel(month: PhotoMonthIndexItem): string {
  return `${month.year}年${month.month}月`;
}
