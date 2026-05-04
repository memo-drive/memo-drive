import type { ReactNode } from "react";
import styles from "./FloatButton.module.css";

type FloatButtonSize = "sm" | "md" | "lg";

export interface FloatButtonProps {
  icon: ReactNode;
  size?: FloatButtonSize;
  bottom?: number | string;
  right?: number | string;
  disabled?: boolean;
  className?: string;
  style?: Record<string, string | number>;
  onClick?: () => void;
  [key: string]: unknown;
}

const sizeClass: Record<FloatButtonSize, string> = {
  sm: styles.sm,
  md: styles.md,
  lg: styles.lg,
};

export function FloatButton({
  icon,
  size = "md",
  bottom = 40,
  right = 40,
  className = "",
  style,
  ...rest
}: FloatButtonProps) {
  const cls = [styles.fab, sizeClass[size], className]
    .filter(Boolean)
    .join(" ");

  return (
    <button className={cls} style={{ bottom, right, ...style }} {...rest}>
      {icon}
    </button>
  );
}
