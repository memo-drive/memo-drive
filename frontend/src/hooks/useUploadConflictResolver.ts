import { useEffect, useRef, useState } from "react";
import { preflightFileConflicts } from "../api/fileApi";
import type { FileConflictPreflightItem } from "../types";
import type { PreparedDirectoryUploadItem } from "../workflows/directoryUploadWorkflow";
import {
  applyUploadConflictDecision,
  conflictOrdinal,
  createUploadConflictPlan,
  nextUnresolvedConflict,
  resolvedUploadItems,
  type ResolvedUploadItem,
  type UploadConflictAction,
  type UploadConflictContext,
} from "../workflows/uploadConflictWorkflow";

export interface PendingUploadConflict {
  file: File;
  item: FileConflictPreflightItem;
  current: number;
  total: number;
}

export interface ResolvedUploadBatch {
  uploads: ResolvedUploadItem[];
  skipped: number;
}

interface ConflictDecision {
  action: UploadConflictAction;
  applyToRemaining: boolean;
}

export function useUploadConflictResolver() {
  const [conflict, setConflict] = useState<PendingUploadConflict | null>(null);
  const decisionRef = useRef<((decision: ConflictDecision) => void) | null>(
    null,
  );
  const resolvingRef = useRef(false);

  useEffect(
    () => () => {
      decisionRef.current?.({
        action: "skip",
        applyToRemaining: true,
      });
      decisionRef.current = null;
    },
    [],
  );

  async function resolve(
    files: File[],
    destPath: string,
  ): Promise<ResolvedUploadBatch> {
    return resolvePlan(async () => {
      const response = await preflightFileConflicts(
        destPath,
        files.map((file) => file.name),
      );
      return { files, items: response.items };
    });
  }

  async function resolvePrepared(
    batchId: string,
    items: PreparedDirectoryUploadItem[],
  ): Promise<ResolvedUploadBatch> {
    return resolvePlan(async () => ({
      files: items.map((item) => item.file),
      items: items.map((item) => item.preflight),
      contexts: items.map<UploadConflictContext>((item) => ({
        batchId,
        relativePath: item.relativePath,
        destPath: item.destPath,
      })),
    }));
  }

  async function resolvePlan(load: () => Promise<{
    files: File[];
    items: FileConflictPreflightItem[];
    contexts?: UploadConflictContext[];
  }>): Promise<ResolvedUploadBatch> {
    if (resolvingRef.current) {
      throw new Error("upload conflict resolution is already active");
    }
    resolvingRef.current = true;
    try {
      const loaded = await load();
      let plan = createUploadConflictPlan(loaded.files, loaded.items, loaded.contexts);
      for (
        let index = nextUnresolvedConflict(plan);
        index >= 0;
        index = nextUnresolvedConflict(plan)
      ) {
        const ordinal = conflictOrdinal(plan, index);
        setConflict({
          file: plan[index].file,
          item: plan[index].preflight,
          ...ordinal,
        });
        const decision = await new Promise<ConflictDecision>((resolveDecision) => {
          decisionRef.current = resolveDecision;
        });
        decisionRef.current = null;
        plan = applyUploadConflictDecision(
          plan,
          index,
          decision.action,
          decision.applyToRemaining,
        );
      }
      const uploads = resolvedUploadItems(plan);
      return { uploads, skipped: loaded.files.length - uploads.length };
    } finally {
      decisionRef.current = null;
      resolvingRef.current = false;
      setConflict(null);
    }
  }

  function decide(
    action: UploadConflictAction,
    applyToRemaining = false,
  ) {
    decisionRef.current?.({ action, applyToRemaining });
  }

  return { conflict, resolve, resolvePrepared, decide };
}
