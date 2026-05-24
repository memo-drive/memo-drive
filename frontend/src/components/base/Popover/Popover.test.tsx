import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { renderToString } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Popover } from "./Popover";

describe("Popover", () => {
  it("does not inline controlled-open content during server rendering", () => {
    const html = renderToString(
      <Popover open content={<button>Rename</button>}>
        <button>More</button>
      </Popover>,
    );

    expect(html).toContain("More");
    expect(html).not.toContain("Rename");
  });

  it("portals menu content to body so transformed virtual rows do not offset fixed positioning", () => {
    const source = readFileSync(resolve(__dirname, "Popover.tsx"), "utf8");

    expect(source).toContain('import { createPortal } from "react-dom";');
    expect(source).toMatch(/createPortal\(\s*popoverNode,\s*document\.body\s*\)/s);
  });
});
