import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import "../../i18n";
import { MobileMoveTargetPrompt } from "./MobileMoveTargetPrompt";

describe("MobileMoveTargetPrompt", () => {
  it("renders current Folder target, child Folder choices, and disabled reason", () => {
    const html = renderToString(
      <MobileMoveTargetPrompt
        open
        title="移动 2 项"
        currentDir="/Docs"
        dirs={[makeFolder({ id: "folder-1", name: "Archive", path: "/Docs" })]}
        disabledReason="文件已经在这个目录中"
        onClose={() => undefined}
        onMoveHere={() => undefined}
        onEnterDir={() => undefined}
        onGoToDir={() => undefined}
      />,
    );

    expect(html).toContain("data-mobile-move-target=\"light\"");
    expect(html).toContain("移动 2 项");
    expect(html).toContain("/Docs");
    expect(html).toContain("Archive");
    expect(html).toContain("文件已经在这个目录中");
    expect(html).toContain("移到这里");
    expect(html).toContain("disabled=\"\"");
  });

  it("offers another cursor page when more target Folders are available", () => {
    const html = renderToString(
      <MobileMoveTargetPrompt
        open
        title="移动文件"
        currentDir="/"
        dirs={[makeFolder()]}
        hasMore
        onClose={() => undefined}
        onMoveHere={() => undefined}
        onEnterDir={() => undefined}
        onGoToDir={() => undefined}
        onLoadMore={() => undefined}
      />,
    );

    expect(html).toContain("加载更多");
  });
});

function makeFolder(overrides: Partial<DriveFile> = {}): DriveFile {
  return {
    id: "folder-1",
    name: "Archive",
    path: "/",
    storage_path: "Archive",
    size: 0,
    mime_type: "inode/directory",
    is_dir: true,
    status: "ready",
    chunk_count: 0,
    created_at: "2026-05-10T00:00:00Z",
    updated_at: "2026-05-10T00:00:00Z",
    ...overrides,
  };
}
