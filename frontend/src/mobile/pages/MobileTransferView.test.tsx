import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { TransferTask } from "../../stores/transferStore";
import "../../i18n";
import { MobileTransferView } from "./MobileTransferView";

describe("MobileTransferView", () => {
  it("renders Upload Session cards with status and progress", () => {
    const html = renderToString(
      <MobileTransferView
        tasks={[
          makeTask({ fileName: "Memo.pdf", status: "uploading", percent: 45 }),
          makeTask({ id: "done-1", fileName: "Done.txt", status: "done", percent: 100 }),
        ]}
      />,
    );

    expect(html).toContain("Memo.pdf");
    expect(html).toContain("45%");
    expect(html).toContain("Done.txt");
    expect(html).toContain("已完成");
  });

  it("renders mobile transfer controls for active and historical sessions", () => {
    const html = renderToString(
      <MobileTransferView
        tasks={[
          makeTask({ id: "active-1", fileName: "Uploading.mov", status: "uploading" }),
          makeTask({ id: "paused-1", fileName: "Paused.zip", status: "paused" }),
          makeTask({ id: "done-1", fileName: "Done.txt", status: "done", percent: 100 }),
        ]}
        onPause={() => undefined}
        onResume={() => undefined}
        onCancel={() => undefined}
        onRemove={() => undefined}
        onClearDone={() => undefined}
      />,
    );

    expect(html).toContain("暂停");
    expect(html).toContain("继续");
    expect(html).toContain("取消");
    expect(html).toContain("删除");
    expect(html).toContain("清除全部");
  });
});

function makeTask(overrides: Partial<TransferTask> = {}): TransferTask {
  return {
    id: "upload-1",
    fileName: "Memo.pdf",
    fileSize: 2048,
    destPath: "/Docs",
    direction: "upload",
    status: "uploading",
    percent: 45,
    uploadedChunks: [0],
    totalChunks: 2,
    chunkSize: 1024,
    speed: 0,
    createdAt: new Date("2026-05-10T00:00:00Z").getTime(),
    updatedAt: new Date("2026-05-10T00:00:00Z").getTime(),
    ...overrides,
  };
}
