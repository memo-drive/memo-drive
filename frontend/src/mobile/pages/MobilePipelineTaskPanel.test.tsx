import { renderToString } from "react-dom/server";
import { expect, it } from "vitest";
import "../../i18n";
import { MobileTransferPage } from "./MobileTransfer";

it("shows pipeline processing Tasks below mobile transfers", () => {
  const html = renderToString(<MobileTransferPage />);

  expect(html).toContain("文件处理任务");
  expect(html).toContain("正在加载处理任务");
});
