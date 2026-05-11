import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import "../../../i18n";
import { MobileTextPrompt } from "./MobileTextPrompt";

describe("MobileTextPrompt", () => {
  it("renders a lightweight mobile text form with value and actions", () => {
    const html = renderToString(
      <MobileTextPrompt
        open
        title="重命名"
        label="新的文件名"
        value="Memo.pdf"
        confirmText="保存"
        cancelText="取消"
        onValueChange={vi.fn()}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(html).toContain("role=\"dialog\"");
    expect(html).toContain("data-mobile-text-prompt=\"light\"");
    expect(html).toContain("重命名");
    expect(html).toContain("新的文件名");
    expect(html).toContain("value=\"Memo.pdf\"");
    expect(html).toContain("保存");
    expect(html).toContain("取消");
  });

  it("renders nothing when closed", () => {
    const html = renderToString(
      <MobileTextPrompt
        open={false}
        title="重命名"
        label="新的文件名"
        value="Memo.pdf"
        confirmText="保存"
        onValueChange={vi.fn()}
        onCancel={vi.fn()}
        onConfirm={vi.fn()}
      />,
    );

    expect(html).toBe("");
  });
});
