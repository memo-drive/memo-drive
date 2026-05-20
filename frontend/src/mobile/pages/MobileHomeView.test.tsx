import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import "../../i18n";
import { MobileHomeView } from "./MobileHomeView";

describe("MobileHomeView", () => {
  it("renders search, transfer entry, category shortcuts, recent files, and upload", () => {
    const html = renderHome(
      <MobileHomeView
        searchDraft=""
        transferCount={3}
        recentFiles={[
          makeFile({ id: "photo-1", name: "旅行.jpg", mime_type: "image/jpeg" }),
          makeFile({ id: "doc-1", name: "计划.pdf", mime_type: "application/pdf" }),
        ]}
      />,
    );

    expect(html).toContain("移动首页");
    expect(html).toContain("placeholder=\"搜索全部文件\"");
    expect(html).toContain("href=\"/m/transfer\"");
    expect(html).toContain("3");
    expect(html).toContain("href=\"/m/category/photos\"");
    expect(html).toContain("照片");
    expect(html).toContain("视频");
    expect(html).toContain("文档");
    expect(html).toContain("音频");
    expect(html).toContain("最近查看");
    expect(html).toContain("旅行.jpg");
    expect(html).toContain("href=\"/m/media/photos/photo-1?returnTo=%2Fm\"");
    expect(html).toContain("href=\"/m/preview/doc-1?path=%2F\"");
    expect(html).toContain("aria-label=\"上传文件\"");
  });

  it("renders flat in-page search results without linking to the Files page search", () => {
    const html = renderHome(
      <MobileHomeView
        searchDraft="旅行"
        searchActive
        searchResults={[
          makeFile({ id: "photo-1", name: "旅行.jpg", mime_type: "image/jpeg" }),
          makeFile({ id: "audio-1", name: "旅行录音.mp3", mime_type: "audio/mpeg" }),
        ]}
        recentFiles={[]}
      />,
    );

    expect(html).toContain("搜索结果");
    expect(html).toContain("旅行.jpg");
    expect(html).toContain("旅行录音.mp3");
    expect(html).not.toContain("/m/files?path=");
  });

  it("uses available thumbnails for recent photos and videos on the home page", () => {
    const html = renderHome(
      <MobileHomeView
        searchDraft=""
        recentFiles={[
          makeFile({
            id: "photo-1",
            name: "IMG_8196.jpeg",
            mime_type: "image/jpeg",
            metadata: thumbnailMeta("photo-1"),
          }),
          makeFile({
            id: "video-1",
            name: "IMG_8158.mov",
            mime_type: "video/quicktime",
            metadata: thumbnailMeta("video-1"),
          }),
        ]}
      />,
    );

    expect(html).toContain("src=\"/api/files/photo-1/thumbnail\"");
    expect(html).toContain("src=\"/api/files/video-1/thumbnail\"");
    expect(html).toContain("alt=\"IMG_8196.jpeg\"");
    expect(html).toContain("alt=\"IMG_8158.mov\"");
  });
});

function renderHome(node: React.ReactNode) {
  return renderToString(<MemoryRouter>{node}</MemoryRouter>);
}

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.pdf",
    path: "/",
    storage_path: "Memo.pdf",
    size: 1024,
    mime_type: "application/pdf",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}

function thumbnailMeta(fileId: string) {
  return {
    file_id: fileId,
    meta_json: "{}",
    thumbnail_path: `thumbs/${fileId}.jpg`,
    extracted_at: "2026-05-10T00:00:00Z",
  };
}
