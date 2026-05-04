import { useEffect, useRef, useState, type ReactNode } from "react";
import styles from "./Popover.module.css";

type Placement =
  | "bottom-start"
  | "bottom"
  | "bottom-end"
  | "top-start"
  | "top"
  | "top-end";

type Trigger = "click" | "hover";

export interface PopoverProps {
  children: ReactNode;
  content: ReactNode;
  placement?: Placement;
  trigger?: Trigger;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  className?: string;
}

interface Position {
  top: number;
  left: number;
  arrowLeft: number;
  actualPlacement: Placement;
}

export function Popover({
  children,
  content,
  placement: preferredPlacement = "bottom-start",
  trigger = "click",
  open: controlledOpen,
  onOpenChange,
  className = "",
}: PopoverProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const isControlled = controlledOpen !== undefined;
  const open = isControlled ? controlledOpen : internalOpen;

  const [pos, setPos] = useState<Position>({
    top: 0,
    left: 0,
    arrowLeft: 0,
    actualPlacement: preferredPlacement,
  });

  const triggerRef = useRef<HTMLSpanElement | null>(null);
  const popoverRef = useRef<HTMLDivElement | null>(null);

  function setOpenVal(v: boolean) {
    if (!isControlled) setInternalOpen(v);
    onOpenChange?.(v);
  }

  function toggle() {
    setOpenVal(!open);
  }

  // ESC + click outside
  useEffect(() => {
    if (!open) return;
    const onKey = (e: Event) => {
      if ((e as KeyboardEvent).key === "Escape") setOpenVal(false);
    };
    const onMouseDown = (e: Event) => {
      const target = e.target as Node;
      if (popoverRef.current?.contains(target)) return;
      if (triggerRef.current?.contains(target)) return;
      setOpenVal(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onMouseDown, true);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onMouseDown, true);
    };
  }, [open]);

  // Position calculation with auto-flip
  function calcPos() {
    if (!triggerRef.current || !popoverRef.current) return;
    const rect = triggerRef.current.getBoundingClientRect();
    const gap = 6;
    const pw = popoverRef.current.offsetWidth;
    const ph = popoverRef.current.offsetHeight;
    const vw = window.innerWidth;
    const vh = window.innerHeight;

    const [prefSide, prefAlign = "start"] = preferredPlacement.split("-") as [
      "top" | "bottom",
      "start" | "center" | "end" | undefined,
    ];

    // Auto-flip: check if preferred side has enough space
    const spaceBelow = vh - rect.bottom - gap;
    const spaceAbove = rect.top - gap;
    let side = prefSide;
    if (prefSide === "bottom" && spaceBelow < ph && spaceAbove > spaceBelow) {
      side = "top";
    } else if (prefSide === "top" && spaceAbove < ph && spaceBelow > spaceAbove) {
      side = "bottom";
    }

    // Calculate top
    const top =
      side === "bottom" ? rect.bottom + gap : rect.top - gap - ph;

    // Calculate left from alignment
    let left: number;
    if (prefAlign === "start") {
      left = rect.left;
    } else if (prefAlign === "end") {
      left = rect.right - pw;
    } else {
      left = rect.left + rect.width / 2 - pw / 2;
    }

    // Arrow position relative to trigger center
    let arrowLeft = rect.left + rect.width / 2 - left;

    // Clamp to viewport
    const margin = 8;
    if (left < margin) {
      arrowLeft += left - margin;
      left = margin;
    }
    if (left + pw > vw - margin) {
      arrowLeft += (left + pw) - (vw - margin);
      left = vw - pw - margin;
    }
    if (top < margin) {
      // This shouldn't happen with auto-flip, but clamp just in case
    }
    if (top + ph > vh - margin) {
      // Same
    }

    // Clamp arrow to popover bounds
    arrowLeft = Math.max(12, Math.min(arrowLeft, pw - 12));

    setPos({
      top,
      left,
      arrowLeft,
      actualPlacement: `${side}-${prefAlign}` as Placement,
    });
  }

  // Recalc position when open
  useEffect(() => {
    if (open) calcPos();
  }, [open]);

  const actualSide = pos.actualPlacement.split("-")[0];
  const arrowCls =
    actualSide === "top" ? styles.arrowTop : styles.arrowBottom;

  // Trigger events
  const triggerHandlers: Record<string, unknown> = {};
  if (trigger === "click") {
    triggerHandlers.onMouseDown = toggle;
  } else if (trigger === "hover") {
    triggerHandlers.onMouseEnter = () => setOpenVal(true);
    triggerHandlers.onMouseLeave = () => setOpenVal(false);
  }

  return (
    <>
      {/* Trigger wrapper */}
      <span ref={triggerRef} className={styles.wrapper} {...triggerHandlers}>
        {children}
      </span>

      {/* Popover content — sibling, not child */}
      {open && (
        <div
          ref={popoverRef}
          className={`${styles.popover} ${styles.enter} ${className}`}
          style={{ top: pos.top, left: pos.left }}
          role="menu"
          onClick={() => setOpenVal(false)}
          onMouseEnter={
            trigger === "hover" ? () => {} : undefined
          }
          onMouseLeave={
            trigger === "hover"
              ? () => setOpenVal(false)
              : undefined
          }
        >
          <span
            className={`${styles.arrow} ${arrowCls}`}
            style={{ left: pos.arrowLeft }}
          />
          {content}
        </div>
      )}
    </>
  );
}
