import type { ReactNode } from "react";
import styles from "./Button.module.css";

type ButtonVariant = "primary" | "secondary" | "danger" | "ghost" | "pill";
type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps {
  variant?: ButtonVariant;
  size?: ButtonSize;
  block?: boolean;
  loading?: boolean;
  disabled?: boolean;
  icon?: ReactNode;
  children?: ReactNode;
  className?: string;
  type?: "button" | "submit" | "reset";
  onClick?: () => void;
  [key: string]: unknown;
}

const sizeClass: Record<ButtonSize, string> = {
  sm: styles.sizeSm,
  md: styles.sizeMd,
  lg: styles.sizeLg,
};

const variantClass: Record<ButtonVariant, string> = {
  primary: styles.primary,
  secondary: styles.secondary,
  danger: styles.danger,
  ghost: styles.ghost,
  pill: styles.pill,
};

export function Button({
  variant = "primary",
  size = "md",
  block = false,
  loading = false,
  disabled,
  icon,
  children,
  className = "",
  type = "button",
  ...rest
}: ButtonProps) {
  const cls = [
    styles.btn,
    variantClass[variant],
    sizeClass[size],
    block ? styles.block : "",
    className,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <button
      className={cls}
      disabled={disabled || loading}
      type={type}
      {...rest}
    >
      {loading ? (
        <span className={styles.spinner} aria-hidden />
      ) : icon ? (
        <span className="btn-icon">{icon}</span>
      ) : null}
      {children}
    </button>
  );
}
