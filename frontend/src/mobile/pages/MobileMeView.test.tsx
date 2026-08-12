import { renderToString } from "react-dom/server";
import { MemoryRouter } from "react-router-dom";
import { describe, expect, it } from "vitest";
import type { StorageUsage } from "../../types";
import "../../i18n";
import { MobileMeView } from "./MobileMeView";

describe("MobileMeView", () => {
  it("renders storage usage and Trash entry", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobileMeView
          storageUsage={{
            used_bytes: 1024,
            total_bytes: 8192,
            active_bytes: 1024,
            trash_bytes: 512,
            version_bytes: 0,
            temp_bytes: 256,
            auxiliary_bytes: 128,
            filesystem_total_bytes: 8192,
            filesystem_available_bytes: 4096,
            quota_bytes: 0,
            reserved_bytes: 1024,
            upload_available_bytes: 3072,
          } as StorageUsage}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("可用于上传");
    expect(html).toContain("3.0 KB");
    expect(html).toContain("文件");
    expect(html).toContain("1.0 KB");
    expect(html).toContain("回收站");
    expect(html).toContain("512 B");
    expect(html).toContain("临时文件");
    expect(html).toContain("256 B");
    expect(html).toContain("磁盘实际剩余");
    expect(html).toContain("4.0 KB");
    expect(html).toContain("href=\"/m/trash\"");
  });

  it("renders language choices and logout action", () => {
    const html = renderToString(
      <MemoryRouter>
        <MobileMeView
          storageUsage={null}
          currentLanguage="zh-CN"
          onLanguageChange={() => undefined}
          onLogout={() => undefined}
		  onLogoutAll={() => undefined}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("语言");
    expect(html).toContain("中文");
    expect(html).toContain("English");
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).toContain("退出登录");
		expect(html).toContain("退出所有设备");
  });
});
