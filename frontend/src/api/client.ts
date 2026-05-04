export {
  httpClient,
  getToken,
  setToken,
  clearToken,
} from "./HttpClient";

import { httpClient } from "./HttpClient";

/** @deprecated use httpClient.get / httpClient.post instead */
export const request = httpClient;

/** @deprecated use httpClient.assetUrl instead */
export function apiUrl(path: string) {
  return httpClient.assetUrl(path).split("?")[0];
}

/** @deprecated use httpClient.assetUrl instead */
export function authedAssetUrl(path: string) {
  return httpClient.assetUrl(path);
}

/** @deprecated use httpClient.login instead */
export async function login(password: string) {
  return httpClient.login(password);
}
