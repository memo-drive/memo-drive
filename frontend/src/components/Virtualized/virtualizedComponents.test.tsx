import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import { LazyThumbnail, VirtualGrid, VirtualList } from "./index";

describe("virtualized file foundations", () => {
  it("renders only the initial virtual window for large fixed-height lists", () => {
    const html = renderToString(
      <VirtualList
        items={Array.from({ length: 100 }, (_, index) => `row-${index}`)}
        height={120}
        estimateSize={40}
        overscan={1}
        renderItem={(item) => <div>{item}</div>}
      />,
    );

    expect(html).toContain('data-virtual-list="true"');
    expect(html).toContain('data-virtual-total-size="4000"');
    expect(html).toContain("row-0");
    expect(html).toContain("row-4");
    expect(html).not.toContain("row-99");
  });

  it("renders virtual grid rows from responsive columns without mounting every thumbnail", () => {
    const html = renderToString(
      <VirtualGrid
        items={Array.from({ length: 50 }, (_, index) => `photo-${index}`)}
        height={220}
        fallbackWidth={320}
        minColumnWidth={100}
        estimateRowHeight={88}
        overscan={1}
        renderItem={(item) => <span>{item}</span>}
      />,
    );

    expect(html).toContain('data-virtual-grid="true"');
    expect(html).toContain('data-virtual-columns="3"');
    expect(html).toContain("photo-0");
    expect(html).toContain("photo-14");
    expect(html).not.toContain("photo-49");
  });

  it("keeps thumbnails request-free until they are mounted near the viewport", () => {
    const file = makeFile({
      id: "photo-1",
      name: "photo.jpg",
      metadata: {
        file_id: "photo-1",
        meta_json: "{}",
        thumbnail_path: "photo-1.jpg",
        extracted_at: "2026-05-19T00:00:00Z",
      },
    });

    const hidden = renderToString(<LazyThumbnail file={file} visible={false} />);
    expect(hidden).not.toContain("/api/files/photo-1/thumbnail");

    const visible = renderToString(<LazyThumbnail file={file} visible />);
    expect(visible).toContain('src="/api/files/photo-1/thumbnail"');
    expect(visible).toContain('loading="lazy"');
    expect(visible).toContain('decoding="async"');
  });
});

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "file.jpg",
    path: "/",
    storage_path: "file.jpg",
    size: 10,
    mime_type: "image/jpeg",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-19T00:00:00Z",
    updated_at: "2026-05-19T00:00:00Z",
    ...overrides,
  };
}
