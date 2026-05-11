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
            total_bytes: 2048,
          } as StorageUsage}
        />
      </MemoryRouter>,
    );

    expect(html).toContain("1.0 KB / 2.0 KB");
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
        />
      </MemoryRouter>,
    );

    expect(html).toContain("语言");
    expect(html).toContain("中文");
    expect(html).toContain("English");
    expect(html).toContain("aria-pressed=\"true\"");
    expect(html).toContain("退出登录");
  });
});
