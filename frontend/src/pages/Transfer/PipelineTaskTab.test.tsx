import { renderToString } from "react-dom/server";
import { expect, it } from "vitest";
import "../../i18n";
import { TransferPage } from "./index";

it("offers pipeline processing alongside active and historical transfers", () => {
  const html = renderToString(<TransferPage />);

  expect(html).toContain("传输中");
  expect(html).toContain("已完成");
  expect(html).toContain("文件处理");
  expect(html).toContain('aria-pressed="false"');
});
