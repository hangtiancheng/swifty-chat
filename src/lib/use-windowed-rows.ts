import { useVirtualizer } from "@tanstack/react-virtual";
import { useRef } from "react";

/**
 * Windows a long table body while keeping real `<table>` semantics: the visible
 * slice is rendered between two spacer rows.
 */
export function useWindowedRows(count: number, rowHeight: number) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const virtualizer = useVirtualizer({
    count,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => rowHeight,
    overscan: 8,
  });

  const items = virtualizer.getVirtualItems();
  const totalSize = virtualizer.getTotalSize();
  const paddingTop = items.length > 0 ? items[0].start : 0;
  const paddingBottom =
    items.length > 0 ? totalSize - items[items.length - 1].end : 0;

  return { scrollRef, items, paddingTop, paddingBottom, totalSize };
}
