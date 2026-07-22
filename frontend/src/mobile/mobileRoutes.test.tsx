import { renderToString } from "react-dom/server";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { describe, expect, it } from "vitest";
import "../i18n";
import { MobileRoutes } from "./MobileRoutes";

describe("mobile routes", () => {
  it("renders the mobile Home entry with five bottom navigation targets", () => {
    const html = renderMobileRoute("/m");

    expect(html).toContain("移动首页");
    expect(html).toContain("href=\"/m\"");
    expect(html).toContain("href=\"/m/files\"");
    expect(html).toContain("href=\"/m/ai\"");
    expect(html).toContain("href=\"/m/transfer\"");
    expect(html).toContain("href=\"/m/me\"");
    expect(html).toContain("aria-label=\"上传文件\"");
  });

  it("renders the mobile Files route with the current Folder path", () => {
    const html = renderMobileRoute("/m/files?path=%2FDocs%2FAI");

    expect(html).toContain("移动文件");
    expect(html).toContain("/Docs/AI");
    expect(html).toContain("新建文件夹");
  });

  it("renders the mobile Preview route without bottom navigation", () => {
    const html = renderMobileRoute("/m/preview/file-1?path=%2FDocs");

    expect(html).toContain("href=\"/m/files?path=%2FDocs\"");
    expect(html).not.toContain("移动预览");
    expect(html).not.toContain("href=\"/m/ai\"");
    expect(html).not.toContain("href=\"/m/transfer\"");
    expect(html).not.toContain("href=\"/m/me\"");
  });

  it("renders media preview deep links without bottom navigation", () => {
    const html = renderMobileRoute("/m/media/photos/photo-1?path=%2F");

    expect(html).toContain("媒体预览");
    expect(html).toContain("href=\"/m/category/photos\"");
    expect(html).not.toContain("href=\"/m/files\"");
    expect(html).not.toContain("aria-label=\"移动端底部导航\"");
  });

  it.each([
    ["/m/category/photos", "照片"],
    ["/m/category/videos", "视频"],
  ])("renders switchable category route %s with its custom tabbar", (path, title) => {
    const html = renderMobileRoute(path);

    expect(html).toContain(title);
    expect(html).toContain("aria-label=\"分类模式导航\"");
    expect(html).not.toContain("aria-label=\"移动端底部导航\"");
  });

  it.each([
    ["/m/category/documents", "文档"],
    ["/m/category/audio", "音频"],
  ])("renders single-mode category route %s without a bottom tabbar", (path, title) => {
    const html = renderMobileRoute(path);

    expect(html).toContain(title);
    expect(html).not.toContain("aria-label=\"分类模式导航\"");
    expect(html).not.toContain("aria-label=\"移动端底部导航\"");
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

  it("renders Mobile AI as a full-screen route with a back button and no bottom nav", () => {
    const html = renderMobileRoute("/m/ai");

    expect(html).toContain("href=\"/m\"");
    expect(html).toContain("aria-label=\"返回\"");
    expect(html).not.toContain("href=\"/m/transfer\"");
    expect(html).not.toContain("aria-label=\"移动端底部导航\"");
  });

  it("omits mobile page eyebrows to preserve vertical space", () => {
    const html = [
      "/m",
      "/m/files",
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
