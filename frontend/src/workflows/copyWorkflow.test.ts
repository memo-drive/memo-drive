import { describe, expect, it } from "vitest";
import type { DriveFile } from "../types";
import { buildCopyRequest } from "./copyWorkflow";

describe("copy workflow", () => {
	it("copies into the selected Folder with a non-destructive rename policy", () => {
		const source = { id: "folder-1", name: "Docs", is_dir: true } as DriveFile;

		expect(buildCopyRequest(source, "/Archive")).toEqual({
			id: "folder-1",
			input: {
				path: "/Archive",
				conflictPolicy: "rename",
			},
		});
	});
});
