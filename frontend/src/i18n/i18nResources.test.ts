import { readFileSync, readdirSync } from "node:fs";
import { extname, join, relative, resolve } from "node:path";
import { describe, expect, it } from "vitest";
import en from "./resources/en.json";
import zhCN from "./resources/zh-CN.json";

const srcRoot = resolve(__dirname, "..");

describe("i18n resources", () => {
  it("covers every static translation key in Chinese and English", () => {
    const usedKeys = staticTranslationKeys(srcRoot);
    const zhKeys = flattenKeys(zhCN);
    const enKeys = flattenKeys(en);

    const missingZh = usedKeys.filter((key) => !zhKeys.has(key));
    const missingEn = usedKeys.filter((key) => !enKeys.has(key));

    expect(missingZh).toEqual([]);
    expect(missingEn).toEqual([]);
  });
});

function staticTranslationKeys(root: string) {
  return Array.from(
    new Set(
      sourceFiles(root)
        .flatMap((file) => {
          const source = readFileSync(file, "utf8");
          return Array.from(source.matchAll(/\bt\(\s*["']([^"']+)["']/g)).map((match) => match[1]);
        })
        .filter((key) => !key.startsWith("http")),
    ),
  ).sort();
}

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name === "dist") return [];
      return sourceFiles(fullPath);
    }
    if (!entry.isFile()) return [];
    if (![".ts", ".tsx"].includes(extname(entry.name))) return [];
    if (relative(srcRoot, fullPath).endsWith("i18nResources.test.ts")) return [];
    return [fullPath];
  });
}

function flattenKeys(value: unknown, prefix = ""): Set<string> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return new Set(prefix ? [prefix] : []);
  }

  return new Set(
    Object.entries(value).flatMap(([key, child]) =>
      Array.from(flattenKeys(child, prefix ? `${prefix}.${key}` : key)),
    ),
  );
}
