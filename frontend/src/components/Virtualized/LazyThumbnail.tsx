import { useState, type ReactNode } from "react";
import { thumbnailUrl } from "../../api/fileApi";
import type { DriveFile } from "../../types";

export interface LazyThumbnailProps {
  file: DriveFile;
  visible?: boolean;
  className?: string;
  placeholderClassName?: string;
  alt?: string;
  fallback?: ReactNode;
}

export function LazyThumbnail({
  file,
  visible = true,
  className,
  placeholderClassName,
  alt,
  fallback,
}: LazyThumbnailProps) {
  const [failed, setFailed] = useState(false);
  const hasThumbnail =
    file.status === "ready" && Boolean(file.metadata?.thumbnail_path);

  if (!visible || !hasThumbnail || failed) {
    return (
      <span className={placeholderClassName} data-thumbnail-placeholder="true" aria-hidden>
        {fallback}
      </span>
    );
  }

  return (
    <img
      className={className}
      src={thumbnailUrl(file.id)}
      alt={alt ?? file.name}
      loading="lazy"
      decoding="async"
      onError={() => setFailed(true)}
    />
  );
}
