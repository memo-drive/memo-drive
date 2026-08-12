import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { StorageUsage } from "../../types";
import "../../i18n";
import { StorageUsagePanel } from "./StorageUsagePanel";

describe("StorageUsagePanel", () => {
  it("shows storage categories, real disk availability, and low-space action", () => {
    const usage: StorageUsage = {
      used_bytes: 600,
      total_bytes: 2000,
      active_bytes: 600,
      trash_bytes: 200,
      version_bytes: 0,
      temp_bytes: 100,
      auxiliary_bytes: 50,
      filesystem_total_bytes: 2000,
      filesystem_available_bytes: 500,
      quota_bytes: 1000,
      reserved_bytes: 100,
      upload_available_bytes: 50,
    };

    const html = renderToString(
      <MemoryRouter>
        <StorageUsagePanel usage={usage} />
      </MemoryRouter>,
    );

    expect(html).toContain("可用于上传");
    expect(html).toContain("50 B");
    expect(html).toContain("文件");
    expect(html).toContain("600 B");
    expect(html).toContain("回收站");
    expect(html).toContain("200 B");
    expect(html).toContain("临时文件");
    expect(html).toContain("100 B");
    expect(html).toContain("磁盘实际剩余");
    expect(html).toContain("500 B");
    expect(html).toContain("可用空间不足");
    expect(html).toContain("href=\"/trash\"");
  });
});
