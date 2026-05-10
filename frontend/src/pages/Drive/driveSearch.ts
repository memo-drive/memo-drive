export interface DriveSearchRequest {
  query: string;
  path: string;
  semantic: boolean;
  limit: number;
}

export const DRIVE_SEARCH_LIMIT = 50;

export function buildDriveSearchRequest(
  query: string,
  currentPath: string,
  semantic: boolean,
): DriveSearchRequest | null {
  const text = query.trim();
  if (!text) return null;
  return {
    query: text,
    path: currentPath,
    semantic,
    limit: DRIVE_SEARCH_LIMIT,
  };
}
