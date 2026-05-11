const DEFAULT_REDIRECT = "/";

function safeRedirectTarget(value: string | null): string {
  const target = value?.trim();
  if (!target) return DEFAULT_REDIRECT;
  if (!target.startsWith("/") || target.startsWith("//")) return DEFAULT_REDIRECT;
  if (target.includes("\\")) return DEFAULT_REDIRECT;
  if (target === "/login" || target.startsWith("/login?") || target.startsWith("/login#")) {
    return DEFAULT_REDIRECT;
  }
  return target;
}

export function loginHrefForRedirect(pathname: string, search: string, hash: string): string {
  const target = safeRedirectTarget(`${pathname}${search}${hash}`);
  return `/login?redirect=${encodeURIComponent(target)}`;
}

export function redirectTargetFromLoginSearch(search: string): string {
  const params = new URLSearchParams(search);
  return safeRedirectTarget(params.get("redirect"));
}
