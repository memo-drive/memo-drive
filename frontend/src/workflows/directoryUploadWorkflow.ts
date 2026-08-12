import type {
  DirectoryPreparedEntry,
  DirectoryPrepareResponse,
  FileConflictPreflightItem,
} from "../types";

export interface LocalDirectoryEntry {
  clientId: string;
  relativePath: string;
  file: File;
}

export function browserSupportsDirectorySelection(input: object): boolean {
  return "webkitdirectory" in input;
}

export interface PreparedDirectoryUploadItem {
  file: File;
  relativePath: string;
  destPath: string;
  preflight: FileConflictPreflightItem;
}

export interface MatchedDirectoryPrepareResult {
  batchId: string;
  uploads: PreparedDirectoryUploadItem[];
  failures: DirectoryPreparedEntry[];
}

export function matchPreparedDirectoryEntries(
  localEntries: LocalDirectoryEntry[],
  response: DirectoryPrepareResponse,
): MatchedDirectoryPrepareResult {
  if (response.entries.length !== localEntries.length) {
    throw new Error("directory prepare response does not match selection");
  }
  const localByID = new Map(localEntries.map((entry) => [entry.clientId, entry]));
  const seen = new Set<string>();
  const uploads: PreparedDirectoryUploadItem[] = [];
  const failures: DirectoryPreparedEntry[] = [];
  for (const entry of response.entries) {
    const local = localByID.get(entry.client_id);
    if (!local || seen.has(entry.client_id)) {
      throw new Error("directory prepare response does not match selection");
    }
    seen.add(entry.client_id);
    if (entry.status !== "ready" || !entry.dest_path || !entry.file_name) {
      failures.push(entry);
      continue;
    }
    uploads.push({
      file: local.file,
      relativePath: local.relativePath,
      destPath: entry.dest_path,
      preflight: {
        requested_name: entry.file_name,
        normalized_name: entry.file_name,
        conflict: entry.conflict,
        existing_file_id: entry.existing_file_id,
        rename_suggestion: entry.rename_suggestion,
        replace_allowed: entry.conflict,
      },
    });
  }
  return { batchId: response.batch_id, uploads, failures };
}

export function selectedDirectoryUploadEntries(
  files: ArrayLike<File>,
): LocalDirectoryEntry[] {
  return Array.from(files).map((file, index) =>
    localDirectoryEntry(file, file.webkitRelativePath, index),
  );
}

interface BrowserFileSystemEntry {
  isFile: boolean;
  isDirectory: boolean;
  fullPath: string;
  file?: (success: (file: File) => void, failure?: (error: DOMException) => void) => void;
  createReader?: () => {
    readEntries: (
      success: (entries: BrowserFileSystemEntry[]) => void,
      failure?: (error: DOMException) => void,
    ) => void;
  };
}

interface BrowserDataTransferItem {
  webkitGetAsEntry?: () => BrowserFileSystemEntry | null;
  getAsFile: () => File | null;
}

export async function droppedDirectoryUploadEntries(
  items: ArrayLike<DataTransferItem>,
): Promise<LocalDirectoryEntry[]> {
  const found: Array<{ file: File; relativePath: string }> = [];
  for (const transferItem of Array.from(items)) {
    const item = transferItem as unknown as BrowserDataTransferItem;
    const entry = item.webkitGetAsEntry?.();
    if (entry) {
      await collectDroppedEntry(entry, found);
      continue;
    }
    const file = item.getAsFile();
    if (file) found.push({ file, relativePath: file.name });
  }
  return found.map((entry, index) =>
    localDirectoryEntry(entry.file, entry.relativePath, index),
  );
}

async function collectDroppedEntry(
  entry: BrowserFileSystemEntry,
  found: Array<{ file: File; relativePath: string }>,
): Promise<void> {
  if (entry.isFile && entry.file) {
    const file = await new Promise<File>((resolve, reject) => entry.file?.(resolve, reject));
    found.push({
      file,
      relativePath: entry.fullPath.replace(/^\/+/, ""),
    });
    return;
  }
  if (!entry.isDirectory || !entry.createReader) return;
  const reader = entry.createReader();
  while (true) {
    const children = await new Promise<BrowserFileSystemEntry[]>((resolve, reject) =>
      reader.readEntries(resolve, reject),
    );
    if (children.length === 0) return;
    for (const child of children) {
      await collectDroppedEntry(child, found);
    }
  }
}

function localDirectoryEntry(
  file: File,
  relativePath: string,
  index: number,
): LocalDirectoryEntry {
  if (!isSafeDirectoryRelativePath(relativePath)) {
    throw new Error(`invalid directory relative path: ${relativePath}`);
  }
  return {
    clientId: `directory-entry-${index}`,
    relativePath,
    file,
  };
}

function isSafeDirectoryRelativePath(relativePath: string): boolean {
  if (
    relativePath.startsWith("/") ||
    /^[A-Za-z]:\//.test(relativePath) ||
    relativePath.includes("\\") ||
    relativePath.includes("\0")
  ) {
    return false;
  }
  const parts = relativePath.split("/");
  return !parts.some((part) => part === "" || part === "." || part === "..");
}
