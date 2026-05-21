import { useEffect, useMemo, useRef, type CSSProperties, type Key, type ReactNode } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { cssSize, estimatedTotalSize, initialVirtualWindow, numericSize } from "./virtualWindow";

export interface VirtualListProps<T> {
  items: T[];
  height?: number | string;
  estimateSize: number;
  overscan?: number;
  hasMore?: boolean;
  isLoading?: boolean;
  endReachedThreshold?: number;
  className?: string;
  itemClassName?: string;
  role?: string;
  getItemKey?: (item: T, index: number) => Key;
  renderItem: (item: T, index: number) => ReactNode;
  onEndReached?: () => void;
}

const DEFAULT_HEIGHT = 480;

export function VirtualList<T>({
  items,
  height = DEFAULT_HEIGHT,
  estimateSize,
  overscan = 4,
  hasMore = false,
  isLoading = false,
  endReachedThreshold = 3,
  className,
  itemClassName,
  role = "list",
  getItemKey,
  renderItem,
  onEndReached,
}: VirtualListProps<T>) {
  const parentRef = useRef<HTMLDivElement | null>(null);
  const requestedForCount = useRef<number | null>(null);
  const viewportHeight = numericSize(height, DEFAULT_HEIGHT);
  const rowVirtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => estimateSize,
    overscan,
    getItemKey: getItemKey
      ? (index) => getItemKey(items[index], index)
      : undefined,
  });
  const measuredItems = rowVirtualizer.getVirtualItems();
  const fallbackItems = useMemo(
    () => initialVirtualWindow(items.length, viewportHeight, estimateSize, overscan),
    [estimateSize, items.length, overscan, viewportHeight],
  );
  const virtualItems = measuredItems.length > 0 ? measuredItems : fallbackItems;
  const totalSize = Math.max(
    rowVirtualizer.getTotalSize(),
    estimatedTotalSize(items.length, estimateSize),
  );

  useEffect(() => {
    if (!hasMore || isLoading || !onEndReached || virtualItems.length === 0) return;
    const last = virtualItems[virtualItems.length - 1];
    if (last.index < items.length - 1 - endReachedThreshold) return;
    if (requestedForCount.current === items.length) return;
    requestedForCount.current = items.length;
    onEndReached();
  }, [endReachedThreshold, hasMore, isLoading, items.length, onEndReached, virtualItems]);

  return (
    <div
      ref={parentRef}
      className={className}
      data-virtual-list="true"
      data-virtual-count={items.length}
      data-virtual-total-size={totalSize}
      role={role}
      style={virtualScrollerStyle(height)}
    >
      <div style={virtualContentStyle(totalSize)}>
        {virtualItems.map((virtualItem) => {
          const item = items[virtualItem.index];
          if (item === undefined) return null;
          const key = getItemKey?.(item, virtualItem.index) ?? virtualItem.key;
          return (
            <div
              key={key}
              className={itemClassName}
              data-virtual-index={virtualItem.index}
              role={role === "list" ? "listitem" : undefined}
              style={virtualRowStyle(virtualItem.start)}
            >
              {renderItem(item, virtualItem.index)}
            </div>
          );
        })}
      </div>
    </div>
  );
}

function virtualScrollerStyle(height: number | string): CSSProperties {
  return {
    height: cssSize(height),
    overscrollBehavior: "contain",
    overflow: "auto",
    position: "relative",
    WebkitOverflowScrolling: "touch",
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

function virtualRowStyle(start: number): CSSProperties {
  return {
    left: 0,
    position: "absolute",
    top: 0,
    transform: `translateY(${start}px)`,
    width: "100%",
  };
}
