import type { DriveFile, FileCopyInput } from "../types";

export interface CopyRequest {
	id: string;
	input: FileCopyInput;
}

export function buildCopyRequest(source: DriveFile, path: string): CopyRequest {
	return {
		id: source.id,
		input: {
			path,
			conflictPolicy: "rename",
		},
	};
}
