import type { DriveFile } from "../../types";

export interface DriveCrumb {
  label: string;
  path: string;
}

export function buildDriveCrumbs(
  currentPath: string,
  rootLabel: string,
  maxLevels: number,
): DriveCrumb[] {
  const parts = currentPath.split("/").filter(Boolean);
  const all: DriveCrumb[] = [
    { label: rootLabel, path: "/" },
    ...parts.map((part, index) => ({
      label: part,
      path: "/" + parts.slice(0, index + 1).join("/"),
    })),
  ];
  if (all.length <= maxLevels) return all;
  return [
    all[0],
    { label: "...", path: "" },
    ...all.slice(all.length - (maxLevels - 1)),
  ];
}

export function driveParentPath(currentPath: string): string | null {
  if (currentPath === "/") return null;
  const parts = currentPath.split("/").filter(Boolean);
  parts.pop();
  return parts.length === 0 ? "/" : "/" + parts.join("/");
}

export function driveFolderPath(file: DriveFile): string {
  return file.path === "/" ? `/${file.name}` : `${file.path}/${file.name}`;
}
