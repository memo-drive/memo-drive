import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import "../../../i18n";
import { MobileConfirmPrompt } from "./MobileConfirmPrompt";

describe("MobileConfirmPrompt", () => {
  it("renders a lightweight destructive confirmation with cancel and confirm actions", () => {
    const html = renderToString(
      <MobileConfirmPrompt
        open
        title="移到回收站"
        description="确定要把 Memo.pdf 移到回收站吗？"
        confirmText="移到回收站"
        cancelText="取消"
        tone="danger"
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(html).toContain("role=\"alertdialog\"");
    expect(html).toContain("data-mobile-confirm=\"light\"");
    expect(html).toContain("移到回收站");
    expect(html).toContain("确定要把 Memo.pdf 移到回收站吗？");
    expect(html).toContain("取消");
  });

  it("renders nothing when closed", () => {
    const html = renderToString(
      <MobileConfirmPrompt
        open={false}
        title="移到回收站"
        description="确定要删除吗？"
        confirmText="移到回收站"
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(html).toBe("");
  });
});
