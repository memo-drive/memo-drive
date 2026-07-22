import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { DriveFile, MediaMeta } from "../../types";
import "../../i18n";
import { MobileMediaPreviewView } from "./MobileMediaPreviewView";

describe("MobileMediaPreviewView", () => {
  it("renders photos as an immersive image preview with swipe navigation and no side buttons", () => {
    const html = renderMedia(
      <MobileMediaPreviewView
        category="photos"
        file={makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" })}
        returnHref="/m/category/photos"
        queuePosition={{ current: 2, total: 8 }}
        canGoPrevious
        canGoNext
        downloadHref="/api/files/photo-1/download"
        meta={{ width: 4032, height: 3024, camera: "GR III" }}
        onPrevious={() => undefined}
        onNext={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-media-kind=\"photo\"");
    expect(html).toContain("src=\"/api/files/photo-1/download\"");
    expect(html).toContain("alt=\"海边.jpg\"");
    expect(html).toContain("2 / 8");
    expect(html).toContain("data-swipe-navigation=\"true\"");
    expect(html).not.toContain("aria-label=\"上一项\"");
    expect(html).not.toContain("aria-label=\"下一项\"");
    expect(html).toContain("aria-label=\"更多\"");
    expect(html).not.toContain("GR III");
    expect(html).not.toContain("4032");
  });

  it("renders videos with native controls, poster, and a file-scoped player key", () => {
    const html = renderMedia(
      <MobileMediaPreviewView
        category="videos"
        file={makeFile({
          id: "video-1",
          name: "课程.mp4",
          mime_type: "video/mp4",
          metadata: { file_id: "video-1", meta_json: "{}", thumbnail_path: "thumbs/video-1.jpg", extracted_at: "2026-05-10T00:00:00Z" },
        })}
        returnHref="/m/category/videos"
        queuePosition={{ current: 1, total: 3 }}
        canGoNext
        downloadHref="/api/files/video-1/download"
        posterHref="/api/files/video-1/thumbnail"
        meta={{ duration: 83, codec: "h264" }}
        onNext={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-media-kind=\"video\"");
    expect(html).toContain("<video");
    expect(html).toContain("controls=\"\"");
    expect(html).toContain("playsInline=\"\"");
    expect(html).toContain("src=\"/api/files/video-1/download\"");
    expect(html).toContain("poster=\"/api/files/video-1/thumbnail\"");
    expect(html).toContain("data-media-player-key=\"video-1\"");
    expect(html).not.toContain("h264");
  });

  it("renders audio with a fixed player and synchronized queue list", () => {
    const queue = [
      makeFile({ id: "audio-1", name: "第一讲.mp3", mime_type: "audio/mpeg" }),
      makeFile({ id: "audio-2", name: "第二讲.mp3", mime_type: "audio/mpeg" }),
    ];

    const html = renderMedia(
      <MobileMediaPreviewView
        category="audio"
        file={queue[1]}
        returnHref="/m/category/audio"
        queue={queue}
        queuePosition={{ current: 2, total: 2 }}
        canGoPrevious
        downloadHref="/api/files/audio-2/download"
        meta={{ duration: 512, codec: "mp3" }}
        onPrevious={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-media-kind=\"audio\"");
    expect(html).toContain("<audio");
    expect(html).toContain("controls=\"\"");
    expect(html).toContain("src=\"/api/files/audio-2/download\"");
    expect(html).toContain("data-audio-queue=\"true\"");
    expect(html).toContain("第一讲.mp3");
    expect(html).toContain("第二讲.mp3");
    expect(html).toContain("aria-current=\"true\"");
    expect(html).not.toContain("mp3</dd>");
  });

  it("keeps single-audio previews from repeating the track title", () => {
    const file = makeFile({ id: "audio-1", name: "qlc的伟大.mp3", mime_type: "audio/mpeg" });
    const html = renderMedia(
      <MobileMediaPreviewView
        category="audio"
        file={file}
        returnHref="/m/category/audio"
        queue={[file]}
        queuePosition={{ current: 1, total: 1 }}
        downloadHref="/api/files/audio-1/download"
      />,
    );

    expect(html).toContain("<h1>音频</h1>");
    expect(html.match(/qlc的伟大\.mp3/g)?.length).toBe(1);
    expect(html).not.toContain("data-audio-queue=\"true\"");
  });

  it("keeps metadata, download, and delete actions inside the more menu", () => {
    const file = makeFile({ id: "photo-1", name: "海边.jpg", mime_type: "image/jpeg" });

    const closed = renderMedia(
      <MobileMediaPreviewView
        category="photos"
        file={file}
        returnHref="/m/category/photos"
        downloadHref="/api/files/photo-1/download"
        meta={{ width: 4032, height: 3024, camera: "GR III" }}
      />,
    );
    const open = renderMedia(
      <MobileMediaPreviewView
        category="photos"
        file={file}
        returnHref="/m/category/photos"
        downloadHref="/api/files/photo-1/download"
        meta={{ width: 4032, height: 3024, camera: "GR III" }}
        moreOpen
        deleteConfirmOpen
        onCloseMore={() => undefined}
        onDelete={() => undefined}
        onCancelDelete={() => undefined}
        onConfirmDelete={() => undefined}
      />,
    );

    expect(closed).not.toContain("GR III");
    expect(closed).not.toContain("4032 x 3024");
    expect(open).toContain("role=\"dialog\"");
    expect(open).toContain("媒体信息");
    expect(open).toContain("4032 x 3024");
    expect(open).toContain("GR III");
    expect(open).toContain("href=\"/api/files/photo-1/download\"");
    expect(open).toContain("下载");
    expect(open).toContain("删除");
    expect(open).toContain("确认删除");
    expect(open).toContain("确定要把 海边.jpg 移到回收站吗？");
  });
});

function renderMedia(node: React.ReactNode) {
  return renderToString(<MemoryRouter>{node}</MemoryRouter>);
}

function makeFile(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "file-1",
    name: "Memo.jpg",
    path: "/",
    storage_path: "Memo.jpg",
    size: 1024,
    mime_type: "image/jpeg",
    is_dir: false,
    status: "ready",
    chunk_count: 1,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
