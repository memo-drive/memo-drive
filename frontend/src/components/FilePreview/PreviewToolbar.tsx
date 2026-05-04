import type { ReactNode } from "react";
import styles from "./FilePreview.module.css";

interface PreviewToolbarProps {
  children?: ReactNode;
  trailing?: ReactNode;
}

export function PreviewToolbar({ children, trailing }: PreviewToolbarProps) {
  return (
    <div className={styles.toolbarInner}>
      <div className={styles.toolbarGroup}>{children}</div>
      {trailing && <div className={styles.toolbarGroup}>{trailing}</div>}
    </div>
  );
}
