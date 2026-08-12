export interface AuthStatus {
  required: boolean;
  authenticated: boolean;
}

export function authStatusAllowsAccess(status: AuthStatus): boolean {
  return !status.required || status.authenticated;
}
