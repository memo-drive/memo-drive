import { describe, expect, it } from "vitest";
import {
  applyUploadConflictDecision,
  conflictOrdinal,
  createUploadConflictPlan,
  nextUnresolvedConflict,
  resolvedUploadItems,
} from "./uploadConflictWorkflow";

describe("uploadConflictWorkflow", () => {
  it("keeps non-conflicting Files on reject and asks only for conflicts", () => {
    const files = [
      new File(["a"], "fresh.txt"),
      new File(["b"], "report.pdf"),
    ];
    const plan = createUploadConflictPlan(files, [
      {
        requested_name: "fresh.txt",
        normalized_name: "fresh.txt",
        conflict: false,
        replace_allowed: false,
      },
      {
        requested_name: "report.pdf",
        normalized_name: "report.pdf",
        conflict: true,
        existing_file_id: "existing",
        rename_suggestion: "report (1).pdf",
        replace_allowed: true,
      },
    ]);

    expect(nextUnresolvedConflict(plan)).toBe(1);
    expect(conflictOrdinal(plan, 1)).toEqual({ current: 1, total: 1 });
    expect(resolvedUploadItems(plan)).toEqual([
      { file: files[0], overwritePolicy: "reject" },
    ]);
  });

  it("applies one decision to all remaining conflicts without changing safe Files", () => {
    const files = [
      new File(["a"], "a.txt"),
      new File(["b"], "b.txt"),
      new File(["c"], "c.txt"),
    ];
    const plan = createUploadConflictPlan(files, [
      {
        requested_name: "a.txt",
        normalized_name: "a.txt",
        conflict: true,
        replace_allowed: true,
      },
      {
        requested_name: "b.txt",
        normalized_name: "b.txt",
        conflict: false,
        replace_allowed: false,
      },
      {
        requested_name: "c.txt",
        normalized_name: "c.txt",
        conflict: true,
        replace_allowed: true,
      },
    ]);

    const decided = applyUploadConflictDecision(
      plan,
      0,
      "rename",
      true,
    );

    expect(nextUnresolvedConflict(decided)).toBe(-1);
    expect(resolvedUploadItems(decided)).toEqual([
      { file: files[0], overwritePolicy: "rename" },
      { file: files[1], overwritePolicy: "reject" },
      { file: files[2], overwritePolicy: "rename" },
    ]);
  });

  it("does not apply Replace to remaining conflicts that cannot be replaced", () => {
    const files = [
      new File(["a"], "a.txt"),
      new File(["b"], "Folder"),
    ];
    const plan = createUploadConflictPlan(files, [
      {
        requested_name: "a.txt",
        normalized_name: "a.txt",
        conflict: true,
        replace_allowed: true,
      },
      {
        requested_name: "Folder",
        normalized_name: "Folder",
        conflict: true,
        replace_allowed: false,
      },
    ]);

    const decided = applyUploadConflictDecision(plan, 0, "replace", true);

    expect(nextUnresolvedConflict(decided)).toBe(1);
  });

  it("removes skipped conflicts from the executable upload plan", () => {
    const files = [new File(["a"], "a.txt")];
    const plan = createUploadConflictPlan(files, [
      {
        requested_name: "a.txt",
        normalized_name: "a.txt",
        conflict: true,
        replace_allowed: true,
      },
    ]);

    const decided = applyUploadConflictDecision(plan, 0, "skip", false);

    expect(resolvedUploadItems(decided)).toEqual([]);
  });

  it("rejects malformed preflight responses instead of uploading unsafely", () => {
    expect(() =>
      createUploadConflictPlan([new File(["a"], "a.txt")], []),
    ).toThrow("does not match selection");
  });

  it("preserves directory context through conflict decisions", () => {
    const file = new File(["a"], "a.txt");
    const plan = createUploadConflictPlan(
      [file],
      [{ requested_name: "a.txt", normalized_name: "a.txt", conflict: true, replace_allowed: true }],
      [{ batchId: "batch-1", relativePath: "Project/a.txt", destPath: "/Docs/Project" }],
    );

    const decided = applyUploadConflictDecision(plan, 0, "rename", false);

    expect(resolvedUploadItems(decided)).toEqual([{
      file,
      overwritePolicy: "rename",
      batchId: "batch-1",
      relativePath: "Project/a.txt",
      destPath: "/Docs/Project",
    }]);
  });
});
