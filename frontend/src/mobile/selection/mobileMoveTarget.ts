import type { DriveFile } from "../../types";
import { normalizeMobilePath } from "../utils/mobilePath";

export type MobileMoveDisabledReason = "alreadyHere" | "cannotMoveToSelf" | "";

export function joinMobileMovePath(base: string, name: string): string {
  return normalizeMobilePath(`${base === "/" ? "" : base}/${name}`);
}

export function mobileMoveDisabledReason(
  targets: DriveFile[],
  currentDir: string,
): MobileMoveDisabledReason {
  if (targets.length === 0) return "";
  const normalized = normalizeMobilePath(currentDir);
  if (targets.every((target) => normalizeMobilePath(target.path || "/") === normalized)) {
    return "alreadyHere";
  }
  return targets.some((target) => target.is_dir && isSelfOrChild(target, normalized))
    ? "cannotMoveToSelf"
    : "";
}

function isSelfOrChild(target: DriveFile, currentDir: string): boolean {
  const targetPath = joinMobileMovePath(target.path || "/", target.name);
  return currentDir === targetPath || currentDir.startsWith(`${targetPath}/`);
}
