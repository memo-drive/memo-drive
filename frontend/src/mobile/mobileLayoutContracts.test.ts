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

  it("keeps the fixed upload button from reducing the Mobile Files list height", () => {
    const pageCss = readMobileCss("pages/MobilePlaceholder.module.css");
    const uploadCss = readMobileCss("components/UploadFab/UploadFab.module.css");
    const filesPage = pageCss.match(/\.page\[data-mobile-page="files"\]\s*{(?<body>[^}]*)}/)?.groups?.body ?? "";

    expect(uploadCss).toMatch(/\.wrap\s*{[^}]*position:\s*fixed/s);
    expect(filesPage).not.toContain("--mobile-upload-fab-clearance");
    expect(filesPage).not.toMatch(/padding-bottom:\s*calc\(var\(--mobile-upload-fab-clearance\)/);
  });

  it("shares bottom navigation and upload clearance tokens across mobile surfaces", () => {
    const tokensCss = readMobileCss("styles/tokens.css");
    const shellCss = readMobileCss("layouts/MobileShell.module.css");
    const uploadCss = readMobileCss("components/UploadFab/UploadFab.module.css");
    const homeCss = readMobileCss("pages/MobileHomeView.module.css");

    expect(tokensCss).toContain("--mobile-bottom-nav-height");
    expect(tokensCss).toContain("--mobile-upload-fab-clearance");
    expect(shellCss).toContain("var(--mobile-bottom-nav-height)");
    expect(uploadCss).toContain("var(--mobile-upload-fab-bottom)");
    expect(homeCss).toContain("var(--mobile-upload-fab-clearance)");
  });

  it("keeps fixed bottom controls ordered and safe-area aware", () => {
    const shellCss = readMobileCss("layouts/MobileShell.module.css");
    const uploadCss = readMobileCss("components/UploadFab/UploadFab.module.css");
    const selectionCss = readMobileCss("selection/MobileSelectionBars.module.css");

    expect(shellCss).toMatch(/\.bottomNav\s*{[^}]*z-index:\s*100;/s);
    expect(shellCss).toMatch(/\.bottomNav\s*{[^}]*env\(safe-area-inset-bottom\)/s);
    expect(uploadCss).toMatch(/\.wrap\s*{[^}]*bottom:\s*var\(--mobile-upload-fab-bottom\);/s);
    expect(uploadCss).toMatch(/\.wrap\s*{[^}]*z-index:\s*120;/s);
    expect(selectionCss).toMatch(/\.batchBar\s*{[^}]*z-index:\s*130;/s);
    expect(selectionCss).toMatch(/\.batchBar\s*{[^}]*env\(safe-area-inset-bottom\)/s);
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
    const page = css.match(/\.page\s*{(?<body>[^}]*)}/)?.groups?.body ?? "";
    const previewArea = css.match(/\.previewArea\s*{(?<body>[^}]*)}/)?.groups?.body ?? "";

    expect(page).toContain("height: 100dvh");
    expect(previewArea).toContain("flex: 1");
    expect(previewArea).toContain("display: flex");
    expect(previewArea).toContain("overflow: auto");
    expect(previewArea).toContain("padding: 0;");
    expect(previewArea).not.toMatch(/padding:\s*0\.75rem/);
    expect(css).toMatch(/\.previewArea\s*>\s*:global\(\*\)\s*{[^}]*flex:\s*1;/s);
    expect(css).toMatch(/\.previewArea\s*>\s*:global\(\*\)\s*{[^}]*height:\s*100%;/s);
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

  it("keeps category filter chips scrollable without showing native scrollbars", () => {
    const css = readMobileCss("pages/MobileCategory.module.css");

    expect(css).toMatch(/\.chipBar\s*{[^}]*overflow-x:\s*auto;/s);
    expect(css).toMatch(/\.chipBar\s*{[^}]*scrollbar-width:\s*none;/s);
    expect(css).toMatch(/\.chipBar::\-webkit-scrollbar\s*{[^}]*display:\s*none;/s);
  });

  it("keeps the video recent rail scrollable without showing native scrollbars", () => {
    const css = readMobileCss("pages/MobileCategory.module.css");

    expect(css).toMatch(/\.recentRail\s*{[^}]*overflow-x:\s*auto;/s);
    expect(css).toMatch(/\.recentRail\s*{[^}]*scrollbar-width:\s*none;/s);
    expect(css).toMatch(/\.recentRail::\-webkit-scrollbar\s*{[^}]*display:\s*none;/s);
  });

  it("keeps Mobile Files as a single virtual-list scroller", () => {
    const pageCss = readMobileCss("pages/MobilePlaceholder.module.css");
    const filesCss = readMobileCss("pages/MobileDriveView.module.css");

    expect(pageCss).toMatch(
      /\.page\[data-mobile-page="files"\]\s*{[^}]*height:\s*calc\(100dvh - var\(--mobile-bottom-nav-height\)\);/s,
    );
    expect(pageCss).toMatch(/\.page\[data-mobile-page="files"\]\s*{[^}]*overflow:\s*hidden;/s);
    expect(pageCss).toMatch(/\.page\[data-mobile-page="files"\]\s*{[^}]*min-height:\s*0;/s);
    expect(filesCss).toMatch(/\.listShell\s*{[^}]*flex:\s*1;/s);
    expect(filesCss).toMatch(/\.listShell\s*{[^}]*min-height:\s*0;/s);
    expect(filesCss).toMatch(/\.list\s*{[^}]*scrollbar-width:\s*none;/s);
    expect(filesCss).toMatch(/\.list\s*{[^}]*overscroll-behavior:\s*contain;/s);
    expect(filesCss).toMatch(/\.list::\-webkit-scrollbar\s*{[^}]*display:\s*none;/s);
  });

  it("keeps photo timeline selection markers separate from hidden photo names", () => {
    const css = readMobileCss("pages/MobileCategory.module.css");

    expect(css).not.toContain(".photoTile span:last-child");
    expect(css).toMatch(/\.photoName\s*{[^}]*color:\s*transparent;/s);
    expect(css).not.toMatch(/\.photoTile\[data-mobile-photo-selected="true"\]\s*{[^}]*outline:/s);
    expect(css).not.toMatch(/\.photoTile\[data-mobile-photo-selected="true"\]::after/s);
    expect(css).not.toMatch(
      /\.photoTile\[data-mobile-photo-selected="true"\]\s+\.categorySelectionMark\s*{[^}]*box-shadow:/s,
    );
  });
});
