import type { DriveFile } from "../../types";

export const EXT_TO_LANG: Record<string, string> = {
  ts: "typescript",
  tsx: "typescript",
  js: "javascript",
  jsx: "javascript",
  go: "go",
  py: "python",
  rs: "rust",
  java: "java",
  cpp: "cpp",
  c: "c",
  h: "c",
  hpp: "cpp",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  toml: "toml",
  sql: "sql",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  md: "markdown",
  markdown: "markdown",
  css: "css",
  scss: "scss",
  html: "xml",
  htm: "xml",
  xml: "xml",
  txt: "plaintext",
  log: "plaintext",
};

const TEXT_LIKE_MIME = new Set([
  "application/json",
  "application/xml",
  "application/javascript",
  "application/x-javascript",
  "application/x-yaml",
  "application/yaml",
  "application/x-sh",
  "application/x-shellscript",
  "application/sql",
]);

export function getFileExtension(fileName: string): string {
  const match = /\.([^.]+)$/.exec(fileName.toLowerCase());
  return match?.[1] ?? "";
}

export function languageForFile(file: DriveFile): string | undefined {
  const ext = getFileExtension(file.name);
  return EXT_TO_LANG[ext];
}

export function isMarkdownFile(file: DriveFile): boolean {
  const ext = getFileExtension(file.name);
  return ext === "md" || ext === "markdown" || file.mime_type === "text/markdown";
}

export function isTextLikeFile(file: DriveFile): boolean {
  const mime = file.mime_type.toLowerCase();
  if (mime.startsWith("text/")) return true;
  if (TEXT_LIKE_MIME.has(mime)) return true;
  return Boolean(languageForFile(file));
}
