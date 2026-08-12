import type { DriveFile } from "../types";

export function appendFolderPage(current: DriveFile[], next: DriveFile[]): DriveFile[] {
  const seen = new Set(current.map((file) => file.id));
  const result = current.slice();
  for (const file of next) {
    if (seen.has(file.id)) continue;
    seen.add(file.id);
    result.push(file);
  }
  return result;
}
