export function mobilePathFromSearch(search: string): string {
  const raw = new URLSearchParams(search).get("path") ?? "/";
  return normalizeMobilePath(raw);
}

export function mobilePreviewReturnHref(search: string): string {
  const params = new URLSearchParams(search);
  const returnTo = params.get("returnTo");
  if (isSafeMobileHref(returnTo)) return returnTo;
  return mobileFilesHref(mobilePathFromSearch(search));
}

export function normalizeMobilePath(value: string): string {
  const trimmed = value.trim();
  if (!trimmed || trimmed === "/") return "/";
  const withSlash = trimmed.startsWith("/") ? trimmed : `/${trimmed}`;
  return withSlash.replace(/\/+/g, "/").replace(/\/$/, "") || "/";
}

export function mobileFilesHref(path: string): string {
  const normalized = normalizeMobilePath(path);
  if (normalized === "/") return "/m/files";
  return `/m/files?path=${encodeURIComponent(normalized)}`;
}

export function mobilePreviewHref(fileId: string, path: string, returnTo?: string): string {
  const params = new URLSearchParams({ path: normalizeMobilePath(path) });
  if (isSafeMobileHref(returnTo ?? null)) params.set("returnTo", returnTo ?? "");
  return `/m/preview/${encodeURIComponent(fileId)}?${params.toString()}`;
}

function isSafeMobileHref(value: string | null): value is string {
  return Boolean(value && (value === "/m" || value.startsWith("/m/")) && !value.startsWith("//"));
}
