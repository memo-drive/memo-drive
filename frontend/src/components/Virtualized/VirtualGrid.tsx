import { useEffect, useMemo, useRef, useState, type CSSProperties, type Key, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { cssSize, estimatedTotalSize, initialVirtualWindow, numericSize } from "./virtualWindow";

export interface VirtualGridProps<T> {
  items: T[];
  height?: number | string;
  estimateRowHeight: number;
  minColumnWidth: number;
  fallbackWidth?: number;
  maxColumns?: number;
  gap?: number;
  overscan?: number;
  hasMore?: boolean;
  isLoading?: boolean;
  endReachedThresholdRows?: number;
  className?: string;
  itemClassName?: string;
  getItemKey?: (item: T, index: number) => Key;
  renderItem: (item: T, index: number) => ReactNode;
  onEndReached?: () => void;
}

const DEFAULT_HEIGHT = 480;
const DEFAULT_WIDTH = 360;

export function VirtualGrid<T>({
  items,
  height = DEFAULT_HEIGHT,
  estimateRowHeight,
  minColumnWidth,
  fallbackWidth = DEFAULT_WIDTH,
  maxColumns,
  gap = 0,
  overscan = 3,
  hasMore = false,
  isLoading = false,
  endReachedThresholdRows = 2,
  className,
  itemClassName,
  getItemKey,
  renderItem,
  onEndReached,
}: VirtualGridProps<T>) {
  const parentRef = useRef<HTMLDivElement | null>(null);
  const requestedForCount = useRef<number | null>(null);
  const measuredWidth = useMeasuredWidth(parentRef, fallbackWidth);
  const columns = columnCountForWidth(measuredWidth, minColumnWidth, gap, maxColumns);
  const rowCount = Math.ceil(items.length / columns);
  const rowSize = estimateRowHeight + gap;
  const viewportHeight = numericSize(height, DEFAULT_HEIGHT);
  const rowVirtualizer = useVirtualizer({
    count: rowCount,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowSize,
    overscan,
  });
  const measuredRows = rowVirtualizer.getVirtualItems();
  const fallbackRows = useMemo(
    () => initialVirtualWindow(rowCount, viewportHeight, rowSize, overscan),
    [overscan, rowCount, rowSize, viewportHeight],
  );
  const virtualRows = measuredRows.length > 0 ? measuredRows : fallbackRows;
  const totalSize = Math.max(
    rowVirtualizer.getTotalSize(),
    estimatedTotalSize(rowCount, estimateRowHeight, gap),
  );

  useEffect(() => {
    if (!hasMore || isLoading || !onEndReached || virtualRows.length === 0) return;
    const last = virtualRows[virtualRows.length - 1];
    if (last.index < rowCount - 1 - endReachedThresholdRows) return;
    if (requestedForCount.current === items.length) return;
    requestedForCount.current = items.length;
    onEndReached();
  }, [endReachedThresholdRows, hasMore, isLoading, items.length, onEndReached, rowCount, virtualRows]);

  return (
    <div
      ref={parentRef}
      className={className}
      data-virtual-grid="true"
      data-virtual-count={items.length}
      data-virtual-columns={columns}
      data-virtual-total-size={totalSize}
      role="grid"
      style={virtualScrollerStyle(height)}
    >
      <div style={virtualContentStyle(totalSize)}>
        {virtualRows.map((virtualRow) => {
          const start = virtualRow.index * columns;
          const rowItems = items.slice(start, start + columns);
          if (rowItems.length === 0) return null;
          return (
            <div
              key={virtualRow.key}
              data-virtual-row={virtualRow.index}
              role="row"
              style={virtualGridRowStyle(virtualRow.start, columns, gap)}
            >
              {rowItems.map((item, offset) => {
                const index = start + offset;
                const key = getItemKey?.(item, index) ?? index;
                return (
                  <div
                    key={key}
                    className={itemClassName}
                    data-virtual-index={index}
                    role="gridcell"
                  >
                    {renderItem(item, index)}
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}

export function columnCountForWidth(
  width: number,
  minColumnWidth: number,
  gap = 0,
  maxColumns?: number,
): number {
  const safeWidth = Math.max(1, width);
  const safeMin = Math.max(1, minColumnWidth);
  const columns = Math.max(1, Math.floor((safeWidth + gap) / (safeMin + gap)));
  return Math.min(columns, maxColumns ?? Number.MAX_SAFE_INTEGER);
}

function useMeasuredWidth(
  ref: React.RefObject<HTMLElement | null>,
  fallbackWidth: number,
): number {
  const [width, setWidth] = useState(fallbackWidth);
  useEffect(() => {
    const element = ref.current;
    if (!element) return;
    const update = () => {
      setWidth(element.clientWidth || fallbackWidth);
    };
    update();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(update);
    observer.observe(element);
    return () => observer.disconnect();
  }, [fallbackWidth, ref]);
  return width;
}

function virtualScrollerStyle(height: number | string): CSSProperties {
  return {
    height: cssSize(height),
    overflow: "auto",
    position: "relative",
    width: "100%",
  };
}

function virtualContentStyle(totalSize: number): CSSProperties {
  return {
    height: `${totalSize}px`,
    position: "relative",
    width: "100%",
  };
}

function virtualGridRowStyle(start: number, columns: number, gap: number): CSSProperties {
  return {
    display: "grid",
    gap: `${gap}px`,
    gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`,
    left: 0,
    position: "absolute",
    top: 0,
    transform: `translateY(${start}px)`,
    width: "100%",
  };
}
