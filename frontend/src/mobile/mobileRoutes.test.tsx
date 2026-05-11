import { renderToString } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import "../i18n";
import { MobileRoutes } from "./MobileRoutes";

describe("mobile routes", () => {
  it("renders the mobile Files entry with bottom navigation", () => {
    const html = renderMobileRoute("/m");

    expect(html).toContain("移动文件");
    expect(html).toContain("href=\"/m\"");
    expect(html).toContain("href=\"/m/ai\"");
    expect(html).toContain("href=\"/m/transfer\"");
    expect(html).toContain("href=\"/m/me\"");
    expect(html).toContain("aria-label=\"上传文件\"");
  });

  it("shows the current Folder path from the mobile Files URL", () => {
    const html = renderMobileRoute("/m?path=%2FDocs%2FAI");

    expect(html).toContain("/Docs/AI");
  });

  it("renders the mobile Preview route without bottom navigation", () => {
    const html = renderMobileRoute("/m/preview/file-1?path=%2FDocs");

    expect(html).toContain("href=\"/m?path=%2FDocs\"");
    expect(html).not.toContain("移动预览");
    expect(html).not.toContain("href=\"/m/ai\"");
    expect(html).not.toContain("href=\"/m/transfer\"");
    expect(html).not.toContain("href=\"/m/me\"");
  });

  it.each([
    ["/m/ai", "移动 AI"],
    ["/m/transfer", "移动传输"],
    ["/m/me", "我的"],
    ["/m/trash", "移动回收站"],
  ])("renders %s", (path, title) => {
    expect(renderMobileRoute(path)).toContain(title);
  });

  it("links Mobile Me to Trash", () => {
    const html = renderMobileRoute("/m/me");

    expect(html).toContain("href=\"/m/trash\"");
  });

  it("omits mobile page eyebrows to preserve vertical space", () => {
    const html = [
      "/m",
      "/m/ai",
      "/m/transfer",
      "/m/me",
      "/m/trash",
      "/m/preview/file-1?path=%2FDocs",
    ]
      .map(renderMobileRoute)
      .join("\n");

    expect(html).not.toContain("MemoDrive H5");
    expect(html).not.toContain("AI Copilot");
    expect(html).not.toContain("Upload Session");
    expect(html).not.toContain("Personal Drive");
    expect(html).not.toContain("Trash Entry");
    expect(html).not.toContain("移动预览");
    expect(html).not.toContain("eyebrow");
  });
});

function renderMobileRoute(path: string) {
  return renderToString(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/m/*" element={<MobileRoutes />} />
      </Routes>
    </MemoryRouter>,
  );
}
