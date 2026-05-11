export function mobilePathFromSearch(search: string): string {
  const raw = new URLSearchParams(search).get("path") ?? "/";
  return normalizeMobilePath(raw);
}

export function normalizeMobilePath(value: string): string {
  const trimmed = value.trim();
  if (!trimmed || trimmed === "/") return "/";
  const withSlash = trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
  return withSlash.replace(/\/+/g, "/").replace(/\/$/, "") || "/";
}

export function mobileFilesHref(path: string): string {
  const normalized = normalizeMobilePath(path);
  if (normalized === "/") return "/m";
  return `/m?path=${encodeURIComponent(normalized)}`;
}

export function mobilePreviewHref(fileId: string, path: string): string {
  return `/m/preview/${encodeURIComponent(fileId)}?path=${encodeURIComponent(
    normalizeMobilePath(path),
  )}`;
}
