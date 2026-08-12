import type {
  FileConflictPolicy,
  FileConflictPreflightItem,
} from "../types";

export type UploadConflictAction = "replace" | "rename" | "skip";
type UploadPlanDecision = FileConflictPolicy | "skip";

export interface UploadConflictPlanItem {
  file: File;
  preflight: FileConflictPreflightItem;
  decision?: UploadPlanDecision;
  context?: UploadConflictContext;
}

export interface UploadConflictContext {
  batchId: string;
  relativePath: string;
  destPath: string;
}

export interface ResolvedUploadItem {
  file: File;
  overwritePolicy: FileConflictPolicy;
  batchId?: string;
  relativePath?: string;
  destPath?: string;
}

export function createUploadConflictPlan(
  files: File[],
  items: FileConflictPreflightItem[],
  contexts?: UploadConflictContext[],
): UploadConflictPlanItem[] {
  if (files.length !== items.length || contexts && contexts.length !== files.length) {
    throw new Error("upload conflict preflight response does not match selection");
  }
  return files.map((file, index) => ({
    file,
    preflight: items[index],
    decision: items[index].conflict ? undefined : "reject",
    context: contexts?.[index],
  }));
}

export function nextUnresolvedConflict(
  plan: UploadConflictPlanItem[],
): number {
  return plan.findIndex(
    (item) => item.preflight.conflict && item.decision === undefined,
  );
}

export function applyUploadConflictDecision(
  plan: UploadConflictPlanItem[],
  index: number,
  decision: UploadConflictAction,
  applyToRemaining: boolean,
): UploadConflictPlanItem[] {
  return plan.map((item, itemIndex) => {
    if (itemIndex === index) {
      return { ...item, decision };
    }
    if (
      applyToRemaining &&
      itemIndex > index &&
      item.preflight.conflict &&
      item.decision === undefined &&
      (decision !== "replace" || item.preflight.replace_allowed)
    ) {
      return { ...item, decision };
    }
    return item;
  });
}

export function resolvedUploadItems(
  plan: UploadConflictPlanItem[],
): ResolvedUploadItem[] {
  return plan.flatMap((item) => {
    switch (item.decision) {
      case "reject":
      case "rename":
      case "replace":
        return [{
          file: item.file,
          overwritePolicy: item.decision,
          ...(item.context ?? {}),
        }];
      case "skip":
      case undefined:
        return [];
    }
  });
}

export function conflictOrdinal(
  plan: UploadConflictPlanItem[],
  index: number,
): { current: number; total: number } {
  const conflicts = plan
    .map((item, itemIndex) => ({ item, itemIndex }))
    .filter(({ item }) => item.preflight.conflict);
  return {
    current: conflicts.findIndex(({ itemIndex }) => itemIndex === index) + 1,
    total: conflicts.length,
  };
}
