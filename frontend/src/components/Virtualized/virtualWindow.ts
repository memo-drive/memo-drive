export interface VirtualWindowItem {
  index: number;
  key: string | number;
  start: number;
  size: number;
}

export function cssSize(value: number | string): number | string {
  return typeof value === "number" ? `${value}px` : value;
}

export function numericSize(value: number | string, fallback: number): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0
    ? value
    : fallback;
}

export function estimatedTotalSize(count: number, estimateSize: number, gap = 0): number {
  if (count <= 0) return 0;
  return count * estimateSize + Math.max(0, count - 1) * gap;
}

export function initialVirtualWindow(
  count: number,
  viewportSize: number,
  estimateSize: number,
  overscan: number,
): VirtualWindowItem[] {
  if (count <= 0) return [];
  const safeEstimate = Math.max(1, estimateSize);
  const visible = Math.ceil(Math.max(1, viewportSize) / safeEstimate);
  const windowCount = Math.min(count, Math.max(1, visible + Math.max(0, overscan) * 2));
  return Array.from({ length: windowCount }, (_, index) => ({
    index,
    key: index,
    start: index * safeEstimate,
    size: safeEstimate,
  }));
}
