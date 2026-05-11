import type { DriveFile } from "../../types";

interface RestoreDeps {
  restore: (id: string) => Promise<unknown>;
  refresh: () => Promise<unknown>;
}

interface PurgeDeps {
  purge: (id: string) => Promise<unknown>;
  refresh: () => Promise<unknown>;
}

interface EmptyDeps {
  empty: () => Promise<{ purged: number }>;
  refresh: () => Promise<unknown>;
}

export async function runMobileTrashRestore(file: DriveFile, deps: RestoreDeps) {
  await deps.restore(file.id);
  await deps.refresh();
}

export async function runMobileTrashPurge(file: DriveFile, deps: PurgeDeps) {
  await deps.purge(file.id);
  await deps.refresh();
}

export async function runMobileTrashEmpty(deps: EmptyDeps) {
  const result = await deps.empty();
  await deps.refresh();
  return result;
}
