import type { ReactNode } from "react";
import styles from "./Segment.module.css";

export interface SegmentOption<T extends string = string> {
  value: T;
  label: ReactNode;
}

interface SegmentProps<T extends string = string> {
  options: SegmentOption<T>[];
  value: T;
  onChange: (value: T) => void;
}

export function Segment<T extends string = string>({
  options,
  value,
  onChange,
}: SegmentProps<T>) {
  return (
    <div className={styles.segment}>
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            className={`${styles.option} ${active ? styles.active : ""}`}
            type="button"
            onClick={() => onChange(opt.value)}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
