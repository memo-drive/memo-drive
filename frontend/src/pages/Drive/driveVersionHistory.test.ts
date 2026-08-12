import { describe, expect, it } from "vitest";
import type { DriveFile } from "../../types";
import { startDriveVersionHistory } from "../../workflows/driveWorkflow";

describe("driveVersionHistory", () => {
	it("opens history for a File and ignores a Folder", () => {
		const file = makeFile(false);
		expect(startDriveVersionHistory(file)).toEqual({ target: file });
		expect(startDriveVersionHistory(makeFile(true))).toBeNull();
	});
});

function makeFile(isDir: boolean): DriveFile {
	return {
		id: isDir ? "folder-1" : "file-1",
		name: isDir ? "Folder" : "memo.md",
		path: "/",
		storage_path: isDir ? "Folder" : "memo.md",
		size: isDir ? 0 : 3,
		mime_type: isDir ? "" : "text/markdown",
		is_dir: isDir,
		status: "ready",
		chunk_count: 0,
		created_at: "2026-08-12T00:00:00Z",
		updated_at: "2026-08-12T00:00:00Z",
	};
}
