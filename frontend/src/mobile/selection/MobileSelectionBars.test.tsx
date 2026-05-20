import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import "../../i18n";
import { MobileBatchActionBar, MobileSelectionTopBar } from "./MobileSelectionBars";

describe("MobileSelectionBars", () => {
  it("renders cancel, selected count, select all, and batch actions", () => {
    const top = renderToString(
      <MobileSelectionTopBar
        selectedCount={2}
        allSelected={false}
        onCancel={() => undefined}
        onSelectAll={() => undefined}
      />,
    );
    const bottom = renderToString(
      <MobileBatchActionBar
        selectedCount={2}
        onMove={() => undefined}
        onDelete={() => undefined}
      />,
    );

    expect(top).toContain("取消");
    expect(top).toContain("已选 2 项");
    expect(top).toContain("全选");
    expect(bottom).toContain("移动");
    expect(bottom).toContain("删除");
    expect(bottom).toContain("data-mobile-batch-bar=\"true\"");
  });

  it("disables batch actions when nothing is selected", () => {
    const html = renderToString(
      <MobileBatchActionBar
        selectedCount={0}
        onMove={() => undefined}
        onDelete={() => undefined}
      />,
    );

    expect(html).toContain("disabled=\"\"");
  });
});
