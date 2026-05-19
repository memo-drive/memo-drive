import { describe, expect, it } from "vitest";
import { resizeFloatingPanelFrame } from "./floatingPanelResize";

describe("floating assistant panel resize", () => {
  it("expands the panel width when dragging the right edge outward", () => {
    const frame = resizeFloatingPanelFrame({
      edge: "right",
      startFrame: { x: 120, y: 80, width: 400, height: 600 },
      dragDelta: { x: 140, y: 45 },
      viewport: { width: 1000, height: 900 },
    });

    expect(frame).toEqual({ x: 120, y: 80, width: 540, height: 600 });
  });

  it("expands the panel height when dragging the bottom edge outward", () => {
    const frame = resizeFloatingPanelFrame({
      edge: "bottom",
      startFrame: { x: 120, y: 80, width: 400, height: 600 },
      dragDelta: { x: 55, y: 120 },
      viewport: { width: 1000, height: 900 },
    });

    expect(frame).toEqual({ x: 120, y: 80, width: 400, height: 720 });
  });

  it("expands from the top-left corner while moving the panel origin", () => {
    const frame = resizeFloatingPanelFrame({
      edge: "top-left",
      startFrame: { x: 220, y: 160, width: 400, height: 600 },
      dragDelta: { x: -70, y: -50 },
      viewport: { width: 1000, height: 900 },
    });

    expect(frame).toEqual({ x: 150, y: 110, width: 470, height: 650 });
  });

  it("keeps the original panel size as the minimum when dragging inward", () => {
    const frame = resizeFloatingPanelFrame({
      edge: "bottom-right",
      startFrame: { x: 120, y: 80, width: 400, height: 600 },
      dragDelta: { x: -120, y: -160 },
      viewport: { width: 1000, height: 900 },
    });

    expect(frame).toEqual({ x: 120, y: 80, width: 400, height: 600 });
  });

  it("keeps the panel inside the viewport when expanding past the top-left", () => {
    const frame = resizeFloatingPanelFrame({
      edge: "top-left",
      startFrame: { x: 20, y: 30, width: 400, height: 600 },
      dragDelta: { x: -100, y: -80 },
      viewport: { width: 1000, height: 900 },
    });

    expect(frame).toEqual({ x: 0, y: 0, width: 420, height: 630 });
  });
});
