import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const editorSource = readFileSync(new URL("./index.tsx", import.meta.url), "utf8");
const editorStyles = readFileSync(new URL("./index.module.css", import.meta.url), "utf8");

describe("MarkdownEditor contracts", () => {
  it("does not use system confirmation dialogs for editor navigation", () => {
    expect(editorSource).not.toContain("window.confirm");
  });

  it("does not expose markdown preview mode in the editor", () => {
    expect(editorSource).not.toContain("ReactMarkdown");
    expect(editorSource).not.toContain("markdownEditor.preview");
    expect(editorSource).not.toContain("role=\"tablist\"");
    expect(editorStyles).not.toContain("previewBody");
  });
});
