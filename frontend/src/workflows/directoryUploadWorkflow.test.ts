import { describe, expect, it } from "vitest";
import {
  browserSupportsDirectorySelection,
  droppedDirectoryUploadEntries,
  matchPreparedDirectoryEntries,
  selectedDirectoryUploadEntries,
} from "./directoryUploadWorkflow";

describe("directoryUploadWorkflow", () => {
  it("preserves webkitRelativePath for every selected File", () => {
    const first = new File(["one"], "one.txt", { type: "text/plain" });
    const second = new File(["two"], "two.txt", { type: "text/plain" });
    Object.defineProperty(first, "webkitRelativePath", {
      value: "Project/one.txt",
    });
    Object.defineProperty(second, "webkitRelativePath", {
      value: "Project/src/two.txt",
    });

    expect(selectedDirectoryUploadEntries([first, second])).toEqual([
      { clientId: "directory-entry-0", relativePath: "Project/one.txt", file: first },
      { clientId: "directory-entry-1", relativePath: "Project/src/two.txt", file: second },
    ]);
  });

  it("rejects unsafe browser relative paths before preparing the batch", () => {
    const file = new File(["unsafe"], "escape.txt", { type: "text/plain" });
    Object.defineProperty(file, "webkitRelativePath", {
      value: "Project/../escape.txt",
    });

    expect(() => selectedDirectoryUploadEntries([file])).toThrow(
      "invalid directory relative path: Project/../escape.txt",
    );
  });

  it("rejects ambiguous or non-portable browser path syntax", () => {
    for (const relativePath of [
      "/Project/a.txt",
      "C:/Project/a.txt",
      "Project\\a.txt",
      "Project//a.txt",
      "Project/./a.txt",
      "Project/a\0.txt",
    ]) {
      const file = new File(["unsafe"], "a.txt");
      Object.defineProperty(file, "webkitRelativePath", { value: relativePath });
      expect(() => selectedDirectoryUploadEntries([file]), relativePath).toThrow(
        `invalid directory relative path: ${relativePath}`,
      );
    }
  });

  it("matches prepared targets to local Files by client id", () => {
    const first = new File(["one"], "one.txt");
    const second = new File(["two"], "two.txt");
    const localEntries = [
      { clientId: "first", relativePath: "Project/one.txt", file: first },
      { clientId: "second", relativePath: "Project/two.txt", file: second },
    ];

    const matched = matchPreparedDirectoryEntries(localEntries, {
      batch_id: "batch-1",
      folders: [],
      entries: [
        {
          client_id: "second",
          relative_path: "Project/two.txt",
          status: "failed",
          conflict: false,
          error: {
            code: "invalid_relative_path",
            message: "invalid",
            retryable: false,
            details: { relative_path: "Project/two.txt", reason: "invalid_name" },
          },
        },
        {
          client_id: "first",
          relative_path: "Project/one.txt",
          dest_path: "/Docs/Project",
          file_name: "one.txt",
          status: "ready",
          conflict: true,
          existing_file_id: "existing-1",
          rename_suggestion: "one (1).txt",
        },
      ],
    });

    expect(matched.batchId).toBe("batch-1");
    expect(matched.uploads).toEqual([{
      file: first,
      relativePath: "Project/one.txt",
      destPath: "/Docs/Project",
      preflight: {
        requested_name: "one.txt",
        normalized_name: "one.txt",
        conflict: true,
        existing_file_id: "existing-1",
        rename_suggestion: "one (1).txt",
		replace_allowed: true,
      },
    }]);
    expect(matched.failures).toEqual([expect.objectContaining({ client_id: "second" })]);
  });

  it("rejects a prepare response that does not cover every local File", () => {
    const file = new File(["one"], "one.txt");
    expect(() => matchPreparedDirectoryEntries(
      [{ clientId: "first", relativePath: "Project/one.txt", file }],
      { batch_id: "batch-1", folders: [], entries: [] },
    )).toThrow("directory prepare response does not match selection");
  });

  it("recursively reads every dropped directory entry batch", async () => {
    const file = new File(["hello"], "main.ts");
    const fileEntry = {
      isFile: true,
      isDirectory: false,
      fullPath: "/Project/src/main.ts",
      file: (success: (value: File) => void) => success(file),
    };
    let subRead = false;
    const subDirectory = {
      isFile: false,
      isDirectory: true,
      fullPath: "/Project/src",
      createReader: () => ({
        readEntries: (success: (entries: unknown[]) => void) => {
          success(subRead ? [] : [fileEntry]);
          subRead = true;
        },
      }),
    };
    let rootRead = false;
    const rootDirectory = {
      isFile: false,
      isDirectory: true,
      fullPath: "/Project",
      createReader: () => ({
        readEntries: (success: (entries: unknown[]) => void) => {
          success(rootRead ? [] : [subDirectory]);
          rootRead = true;
        },
      }),
    };
    const item = {
      webkitGetAsEntry: () => rootDirectory,
      getAsFile: () => null,
    } as unknown as DataTransferItem;

    await expect(droppedDirectoryUploadEntries([item])).resolves.toEqual([{
      clientId: "directory-entry-0",
      relativePath: "Project/src/main.ts",
      file,
    }]);
  });

  it("detects directory selection support from the browser input", () => {
    expect(browserSupportsDirectorySelection({ webkitdirectory: false })).toBe(true);
    expect(browserSupportsDirectorySelection({})).toBe(false);
  });
});
