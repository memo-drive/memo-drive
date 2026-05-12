import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const mobileRoot = resolve(__dirname);

function readMobileCss(relativePath: string) {
  return readFileSync(resolve(mobileRoot, relativePath), "utf8");
}

function listMobileCssFiles(dir = mobileRoot): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const fullPath = resolve(dir, entry.name);
    if (entry.isDirectory()) {
      return listMobileCssFiles(fullPath);
    }
    return entry.isFile() && entry.name.endsWith(".css") ? [fullPath] : [];
  });
}

function targetsFormControl(selector: string) {
  return /(^|[\s>+~,])(input|textarea|select)([\s.#:[>+~,]|$)/.test(selector);
}

function mobileControlFontSizePx(value: string) {
  const trimmed = value.trim();
  const match = trimmed.match(/^([0-9]*\.?[0-9]+)(px|rem)$/);
  if (!match) return null;

  const amount = Number(match[1]);
  return match[2] === "rem" ? amount * 16 : amount;
}

describe("mobile layout contracts", () => {
  it("uses dynamic viewport height instead of legacy viewport height", () => {
    const cssFiles = [
      "layouts/MobileShell.module.css",
      "pages/MobileAIView.module.css",
      "pages/MobilePreviewView.module.css",
    ];

    for (const file of cssFiles) {
      const css = readMobileCss(file);

      expect(css, file).toContain("100dvh");
      expect(css, file).not.toMatch(/(?<!d)100vh/);
    }
  });

  it("keeps the full-screen AI header below the mobile safe area", () => {
    const css = readMobileCss("pages/MobileAIView.module.css");

    expect(css).toContain("env(safe-area-inset-top)");
    expect(css).toMatch(/\.header\s*{[^}]*padding:\s*calc\(1rem \+ env\(safe-area-inset-top\)\)/s);
  });

  it("reserves scroll clearance below Mobile Files for the fixed upload button", () => {
    const pageCss = readMobileCss("pages/MobilePlaceholder.module.css");
    const uploadCss = readMobileCss("components/UploadFab/UploadFab.module.css");

    expect(uploadCss).toMatch(/\.wrap\s*{[^}]*position:\s*fixed/s);
    expect(pageCss).toMatch(
      /\.page\[data-mobile-page="files"\]\s*{[^}]*padding-bottom:\s*var\(--mobile-upload-fab-clearance\)/s,
    );
  });

  it("shares bottom navigation and upload clearance tokens across mobile surfaces", () => {
    const tokensCss = readMobileCss("styles/tokens.css");
    const shellCss = readMobileCss("layouts/MobileShell.module.css");
    const uploadCss = readMobileCss("components/UploadFab/UploadFab.module.css");
    const pageCss = readMobileCss("pages/MobilePlaceholder.module.css");

    expect(tokensCss).toContain("--mobile-bottom-nav-height");
    expect(tokensCss).toContain("--mobile-upload-fab-clearance");
    expect(shellCss).toContain("var(--mobile-bottom-nav-height)");
    expect(uploadCss).toContain("var(--mobile-upload-fab-bottom)");
    expect(pageCss).toContain("var(--mobile-upload-fab-clearance)");
  });

  it("lets Mobile AI occupy the full viewport without bottom-navigation inset", () => {
    const css = readMobileCss("pages/MobileAIView.module.css");

    expect(css).toMatch(/\.page\s*{[^}]*inset:\s*0;/s);
    expect(css).toMatch(/\.page\s*{[^}]*height:\s*100dvh;/s);
    expect(css).toMatch(/\.inputBar\s*{[^}]*bottom:\s*0;/s);
    expect(css).not.toContain("var(--mobile-bottom-nav-height)");
  });

  it("lets the mobile preview content fill the center viewport without inset spacing", () => {
    const css = readMobileCss("pages/MobilePreviewView.module.css");
    const previewArea = css.match(/\.previewArea\s*{(?<body>[^}]*)}/)?.groups?.body ?? "";

    expect(previewArea).toContain("flex: 1");
    expect(previewArea).toContain("overflow: auto");
    expect(previewArea).toContain("padding: 0;");
    expect(previewArea).not.toMatch(/padding:\s*0\.75rem/);
  });

  it("keeps mobile form controls at 16px or larger so iOS browsers do not auto-zoom on focus", () => {
    const tooSmallControls = listMobileCssFiles().flatMap((file) => {
      const css = readFileSync(file, "utf8");
      return [...css.matchAll(/([^{}]+){([^{}]+)}/g)].flatMap(([, selector, body]) => {
        if (!targetsFormControl(selector)) return [];

        const fontSize = body.match(/font-size:\s*([^;]+);/);
        if (!fontSize) return [];

        const fontSizePx = mobileControlFontSizePx(fontSize[1]);
        if (fontSizePx === null || fontSizePx >= 16) return [];

        return [`${file.replace(`${mobileRoot}/`, "")}: ${selector.trim()} -> ${fontSize[1].trim()}`];
      });
    });

    expect(tooSmallControls).toEqual([]);
  });
});
