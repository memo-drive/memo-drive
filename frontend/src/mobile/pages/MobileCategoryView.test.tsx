import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile, PhotoMonthIndexItem } from "../../types";
import "../../i18n";
import { MobileCategoryView } from "./MobileCategoryView";

describe("MobileCategoryView", () => {
  it("renders a collapsed category search icon and a cancel action for an empty expanded search form", () => {
    const collapsed = renderCategory(
      <MobileCategoryView category="documents" files={[makeFile({ id: "doc-1", name: "计划.pdf" })]} />,
    );
    const emptyExpanded = renderCategory(
      <MobileCategoryView
        category="documents"
        searchOpen
        files={[makeFile({ id: "doc-1", name: "计划.pdf" })]}
      />,
    );

    expect(collapsed).toContain("aria-label=\"搜索\"");
    expect(collapsed).not.toContain("placeholder=\"搜索文档\"");
    expect(emptyExpanded).toContain("placeholder=\"搜索文档\"");
    expect(emptyExpanded).toContain("取消");
    expect(emptyExpanded).not.toContain("aria-label=\"清除搜索\"");
    expect(emptyExpanded).not.toContain(">搜索</button>");
    expect(emptyExpanded).not.toContain("/m/files?path=");
  });

  it("replaces cancel with a compact clear button once the category search has content", () => {
    const expanded = renderCategory(
      <MobileCategoryView
        category="documents"
        searchOpen
        searchDraft="计划"
        searchActive
        files={[makeFile({ id: "doc-1", name: "计划.pdf" })]}
      />,
    );

    expect(expanded).toContain("placeholder=\"搜索文档\"");
    expect(expanded).toContain("value=\"计划\"");
    expect(expanded).toContain("aria-label=\"清除搜索\"");
    expect(expanded).toContain("close");
    expect(expanded).not.toContain(">取消</button>");
    expect(expanded).not.toContain(">清除搜索</button>");
    expect(expanded).not.toContain(">搜索</button>");
  });

  it("renders the photo timeline with month headers, virtual grid, quick month tool, and list tab", () => {
    const months: PhotoMonthIndexItem[] = [
      { year: 2026, month: 5, count: 3 },
      { year: 2026, month: 4, count: 1 },
    ];
    const html = renderCategory(
      <MobileCategoryView
        category="photos"
        photoMode="timeline"
        photoMonths={months}
        activePhotoMonth={months[0]}
        timelineFiles={[
          makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" }),
          makeFile({ id: "photo-2", name: "日落.jpg", mime_type: "image/jpeg" }),
        ]}
      />,
    );

    expect(html).toContain("照片");
    expect(html).toContain("2026年5月");
    expect(html).not.toContain(">2026</span><h2>2026年5月</h2>");
    expect(html).toContain("3 张");
    expect(html).toContain("data-virtual-grid=\"true\"");
    expect(html).toContain("海边.jpg");
    expect(html).toContain("href=\"/m/media/photos/photo-1?returnTo=%2Fm%2Fcategory%2Fphotos\"");
    expect(html).toContain("aria-label=\"月份快速定位\"");
    expect(html).toContain("时光轴");
    expect(html).toContain("列表");
  });

  it("renders the photo list mode with virtual list while preserving the search draft", () => {
    const html = renderCategory(
      <MobileCategoryView
        category="photos"
        photoMode="list"
        searchOpen
        searchDraft="海边"
        searchActive
        files={[makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" })]}
      />,
    );

    expect(html).toContain("value=\"海边\"");
    expect(html).toContain("data-virtual-list=\"true\"");
    expect(html).toContain("海边.jpg");
    expect(html).toContain("href=\"/m/media/photos/photo-1?returnTo=%2Fm%2Fcategory%2Fphotos\"");
  });

  it("renders video recent watched, duration chips, and all videos as a virtual list", () => {
    const html = renderCategory(
      <MobileCategoryView
        category="videos"
        videoFilter="1_10m"
        recentFiles={[makeFile({ id: "video-recent", name: "刚看过.mp4", mime_type: "video/mp4" })]}
        files={[makeFile({ id: "video-1", name: "课程.mp4", mime_type: "video/mp4", size: 188743680 })]}
      />,
    );

    expect(html).toContain("最近观看");
    expect(html).not.toMatch(/<header><h2>最近观看<\/h2><button[^>]*>最近观看<\/button><\/header>/);
    expect(html).toContain("刚看过.mp4");
    expect(html).toContain("1-10分钟");
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).toContain("data-virtual-list=\"true\"");
    expect(html).toContain("href=\"/m/media/videos/video-1?returnTo=%2Fm%2Fcategory%2Fvideos\"");
  });

  it("renders document subtype filters and opens documents through the normal preview route", () => {
    const html = renderCategory(
      <MobileCategoryView
        category="documents"
        documentSubtype="pdf"
        files={[makeFile({ id: "doc-1", name: "手册.pdf", mime_type: "application/pdf" })]}
      />,
    );

    expect(html).toContain("PDF");
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).toContain("data-virtual-list=\"true\"");
    expect(html).toContain("href=\"/m/preview/doc-1?path=%2F&amp;returnTo=%2Fm%2Fcategory%2Fdocuments\"");
    expect(html).not.toContain("aria-label=\"分类模式导航\"");
    expect(html).not.toContain("全部文档");
  });

  it("renders audio sort controls and opens audio through the media preview route", () => {
    const html = renderCategory(
      <MobileCategoryView
        category="audio"
        audioSort="name"
        files={[makeFile({ id: "audio-1", name: "讲座.mp3", mime_type: "audio/mpeg" })]}
      />,
    );

    expect(html).toContain("按名称");
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).toContain("data-virtual-list=\"true\"");
    expect(html).toContain("讲座.mp3");
    expect(html).toContain("href=\"/m/media/audio/audio-1?returnTo=%2Fm%2Fcategory%2Faudio\"");
    expect(html).not.toContain("aria-label=\"分类模式导航\"");
    expect(html).not.toContain("全部音频");
  });

  it("renders selection chrome for category items and replaces the category tabbar", () => {
    const html = renderCategory(
      <MobileCategoryView
        category="photos"
        photoMode="list"
        files={[makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" })]}
        selectionActive
        selectedIds={["photo-1"]}
        selectedCount={1}
        allSelected={false}
        onCancelSelection={() => undefined}
        onSelectAll={() => undefined}
        onBatchMove={() => undefined}
        onBatchDelete={() => undefined}
        onToggleSelection={() => undefined}
        onLongPressFile={() => undefined}
      />,
    );

    expect(html).toContain("已选 1 项");
    expect(html).toContain("data-mobile-batch-bar=\"true\"");
    expect(html).toContain("data-mobile-category-selectable=\"true\"");
    expect(html).toContain("aria-selected=\"true\"");
    expect(html).not.toContain("placeholder=\"搜索照片\"");
    expect(html).not.toContain("aria-label=\"分类模式导航\"");
  });

  it("gives selected photo timeline tiles an explicit selected visual state", () => {
    const months: PhotoMonthIndexItem[] = [{ year: 2026, month: 5, count: 2 }];
    const html = renderCategory(
      <MobileCategoryView
        category="photos"
        photoMode="timeline"
        photoMonths={months}
        activePhotoMonth={months[0]}
        timelineFiles={[
          makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" }),
          makeFile({ id: "photo-2", name: "日落.jpg", mime_type: "image/jpeg" }),
        ]}
        selectionActive
        selectedIds={["photo-1"]}
        selectedCount={1}
        onToggleSelection={() => undefined}
        onLongPressFile={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-photo-selected=\"true\"");
    expect(html).toContain("data-mobile-photo-selected=\"false\"");
    expect(html).toContain(">check</span>");
    expect(html).not.toContain("check_circle");
    expect(html).not.toContain("radio_button_unchecked");
  });
});

function renderCategory(node: React.ReactNode) {
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
