export const FLOATING_PANEL_MIN_WIDTH = 400;
export const FLOATING_PANEL_MIN_HEIGHT = 600;

export type FloatingPanelResizeEdge =
  | "top"
  | "right"
  | "bottom"
  | "left"
  | "top-left"
  | "top-right"
  | "bottom-right"
  | "bottom-left";

export interface FloatingPanelFrame {
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface FloatingPanelResizeInput {
  edge: FloatingPanelResizeEdge;
  startFrame: FloatingPanelFrame;
  dragDelta: {
    x: number;
    y: number;
  };
  viewport: {
    width: number;
    height: number;
  };
  minWidth?: number;
  minHeight?: number;
}

export function resizeFloatingPanelFrame({
  edge,
  startFrame,
  dragDelta,
  viewport,
  minWidth = FLOATING_PANEL_MIN_WIDTH,
  minHeight = FLOATING_PANEL_MIN_HEIGHT,
}: FloatingPanelResizeInput): FloatingPanelFrame {
  let { x, y, width, height } = startFrame;

  if (edge.includes("left")) {
    const requestedX = x + dragDelta.x;
    const maxX = x + width - minWidth;
    const nextX = clamp(requestedX, 0, maxX);
    width += x - nextX;
    x = nextX;
  }

  if (edge.includes("right")) {
    width += dragDelta.x;
  }

  if (edge.includes("top")) {
    const requestedY = y + dragDelta.y;
    const maxY = y + height - minHeight;
    const nextY = clamp(requestedY, 0, maxY);
    height += y - nextY;
    y = nextY;
  }

  if (edge.includes("bottom")) {
    height += dragDelta.y;
  }

  width = clamp(width, minWidth, Math.max(minWidth, viewport.width - x));
  height = clamp(height, minHeight, Math.max(minHeight, viewport.height - y));

  return { x, y, width, height };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}
