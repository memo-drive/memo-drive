import { useEffect, useRef, useState, type PointerEvent } from "react";
import { useTranslation } from "react-i18next";
import { useAIChat } from "../../hooks/useAIChat";
import { AssistantPane } from "./AssistantPane";
import {
  FLOATING_PANEL_MIN_HEIGHT,
  FLOATING_PANEL_MIN_WIDTH,
  resizeFloatingPanelFrame,
  type FloatingPanelFrame,
  type FloatingPanelResizeEdge,
} from "./floatingPanelResize";
import styles from "./AIFloatyBall.module.css";

const DRAG_THRESHOLD = 5;
const RESIZE_HANDLES: { edge: FloatingPanelResizeEdge; className: string }[] = [
  { edge: "top", className: styles.resizeTop },
  { edge: "right", className: styles.resizeRight },
  { edge: "bottom", className: styles.resizeBottom },
  { edge: "left", className: styles.resizeLeft },
  { edge: "top-left", className: styles.resizeTopLeft },
  { edge: "top-right", className: styles.resizeTopRight },
  { edge: "bottom-right", className: styles.resizeBottomRight },
  { edge: "bottom-left", className: styles.resizeBottomLeft },
];

export function AIFloatyBall() {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const { stop } = useAIChat();

  const [pos, setPos] = useState(() => ({
    x: window.innerWidth - 80,
    y: window.innerHeight - 200,
  }));
  const [panelFrame, setPanelFrame] = useState<FloatingPanelFrame>(() => ({
    x: Math.max(window.innerWidth - FLOATING_PANEL_MIN_WIDTH - 40, 20),
    y: 200,
    width: FLOATING_PANEL_MIN_WIDTH,
    height: FLOATING_PANEL_MIN_HEIGHT,
  }));

  const dragging = useRef(false);
  const dragStart = useRef({ x: 0, y: 0 });
  const resizeStart = useRef({ x: 0, y: 0 });
  const resizeStartFrame = useRef<FloatingPanelFrame>(panelFrame);
  const dragTarget = useRef<"ball" | "panel" | "resize">("ball");
  const resizeEdge = useRef<FloatingPanelResizeEdge>("right");
  const moved = useRef(false);
  const posRef = useRef(pos);
  const panelFrameRef = useRef(panelFrame);
  posRef.current = pos;
  panelFrameRef.current = panelFrame;

  function openPanel() {
    const ball = posRef.current;
    const pw = panelFrameRef.current.width;
    const ph = panelFrameRef.current.height;
    let px = ball.x - pw / 2 + 28;
    let py = ball.y - ph - 16;
    px = Math.max(12, Math.min(px, window.innerWidth - pw - 12));
    py = Math.max(12, Math.min(py, window.innerHeight - ph - 12));
    setPanelFrame((frame) => ({ ...frame, x: px, y: py }));
    setExpanded(true);
  }

  function onPointerDown(
    event: PointerEvent,
    target: "ball" | "panel" | "resize",
    edge?: FloatingPanelResizeEdge,
  ) {
    event.stopPropagation();
    if (target === "resize") {
      event.preventDefault();
    }
    (event.target as HTMLElement).setPointerCapture?.(event.pointerId);
    dragging.current = true;
    dragTarget.current = target;
    if (edge) {
      resizeEdge.current = edge;
      resizeStart.current = { x: event.clientX, y: event.clientY };
      resizeStartFrame.current = panelFrameRef.current;
    }
    moved.current = false;
    dragStart.current = { x: event.clientX, y: event.clientY };
  }

  function onPointerMove(event: PointerEvent) {
    if (!dragging.current) return;
    const dx = event.clientX - dragStart.current.x;
    const dy = event.clientY - dragStart.current.y;
    if (Math.abs(dx) > DRAG_THRESHOLD || Math.abs(dy) > DRAG_THRESHOLD) {
      moved.current = true;
    }
    if (!moved.current) return;

    if (dragTarget.current === "ball") {
      const cur = posRef.current;
      setPos({
        x: Math.max(0, Math.min(cur.x + dx, window.innerWidth - 56)),
        y: Math.max(0, Math.min(cur.y + dy, window.innerHeight - 56)),
      });
      dragStart.current = { x: event.clientX, y: event.clientY };
    } else if (dragTarget.current === "panel") {
      const cur = panelFrameRef.current;
      setPanelFrame({
        ...cur,
        x: Math.max(0, Math.min(cur.x + dx, window.innerWidth - cur.width)),
        y: Math.max(0, Math.min(cur.y + dy, window.innerHeight - 60)),
      });
      dragStart.current = { x: event.clientX, y: event.clientY };
    } else {
      const start = resizeStart.current;
      setPanelFrame(
        resizeFloatingPanelFrame({
          edge: resizeEdge.current,
          startFrame: resizeStartFrame.current,
          dragDelta: {
            x: event.clientX - start.x,
            y: event.clientY - start.y,
          },
          viewport: {
            width: window.innerWidth,
            height: window.innerHeight,
          },
        }),
      );
    }
  }

  function onPointerUp() {
    if (!dragging.current) return;
    dragging.current = false;
    if (!moved.current && dragTarget.current === "ball") {
      openPanel();
    }
  }

  useEffect(() => {
    return () => {
      dragging.current = false;
      stop();
    };
  }, [stop]);

  return (
    <>
      {!expanded && (
        <div
          className={styles.ball}
          style={{ top: pos.y, left: pos.x }}
          onPointerDown={(event) => onPointerDown(event, "ball")}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          role="button"
          aria-label={t("ai.openAssistant")}
        >
          <span className={styles.pulseRing} />
          <span className={`material-symbols-outlined ${styles.ballIcon}`}>
            psychiatry
          </span>
        </div>
      )}

      {expanded && (
        <div
          className={styles.panel}
          style={{
            top: panelFrame.y,
            left: panelFrame.x,
            width: panelFrame.width,
            height: panelFrame.height,
          }}
        >
          <AssistantPane
            floating
            onClose={() => setExpanded(false)}
            headerProps={{
              onPointerDown: (event: PointerEvent<HTMLDivElement>) =>
                onPointerDown(event, "panel"),
              onPointerMove,
              onPointerUp,
            }}
          />
          {RESIZE_HANDLES.map((handle) => (
            <span
              key={handle.edge}
              className={`${styles.resizeHandle} ${handle.className}`}
              onPointerDown={(event) =>
                onPointerDown(event, "resize", handle.edge)
              }
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              aria-label={t("ai.resizePanel")}
              role="separator"
            />
          ))}
        </div>
      )}
    </>
  );
}
