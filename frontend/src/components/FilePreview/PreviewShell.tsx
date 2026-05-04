import type { ReactNode } from "react";
import styles from "./FilePreview.module.css";

interface PreviewShellProps {
  children: ReactNode;
  toolbar?: ReactNode;
  meta?: ReactNode;
  mode?: "fullbleed" | "padded";
  className?: string;
  bodyClassName?: string;
}

function cx(...classes: Array<string | false | undefined>) {
  return classes.filter(Boolean).join(" ");
}

export function PreviewShell({
  children,
  toolbar,
  meta,
  mode = "padded",
  className,
  bodyClassName,
}: PreviewShellProps) {
  return (
    <section className={cx(styles.shell, className)}>
      {toolbar && <div className={styles.toolbar}>{toolbar}</div>}
      <div className={styles.content}>
        <div
          className={cx(
            styles.body,
            mode === "fullbleed" ? styles.bodyFullbleed : styles.bodyPadded,
            bodyClassName,
          )}
        >
          {children}
        </div>
        {meta && <aside className={styles.metaPanel}>{meta}</aside>}
      </div>
    </section>
  );
}
