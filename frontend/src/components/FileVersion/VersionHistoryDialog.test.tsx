import { renderToString } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { DriveFile, FileVersion } from "../../types";
import "../../i18n";
import { confirmAndDeleteVersion, VersionHistoryDialog, VersionHistoryList } from "./VersionHistoryDialog";

describe("VersionHistoryList", () => {
	it("shows the metadata and actions for each historical File Version", () => {
		const file = makeFile();
		const version: FileVersion = {
			id: "version 3",
			file_id: file.id,
			version_no: 3,
			size: 1024,
			mime_type: "text/markdown",
			sha256: "a".repeat(64),
			checksum_status: "recorded",
			source: "markdown_save",
			created_at: "2026-08-12T02:00:00Z",
		};
		const html = renderToString(
			<VersionHistoryList
				file={file}
				versions={[version]}
				onRestore={vi.fn()}
				onDelete={vi.fn()}
			/>,
		);
		expect(html).toContain("版本 3");
		expect(html).toContain("Markdown 保存");
		expect(html).toContain("校验和已记录");
		expect(html).toContain("1 KB");
		expect(html).toContain('/api/files/file%201/versions/version%203/download');
		expect(html).toContain("下载");
		expect(html).toContain("恢复");
		expect(html).toContain("删除");
	});
});

describe("confirmAndDeleteVersion", () => {
	it("deletes only after explicit confirmation", async () => {
		const remove = vi.fn().mockResolvedValue(undefined);
		const refresh = vi.fn().mockResolvedValue(undefined);
		await confirmAndDeleteVersion(() => false, remove, refresh);
		expect(remove).not.toHaveBeenCalled();
		expect(refresh).not.toHaveBeenCalled();
		await confirmAndDeleteVersion(() => true, remove, refresh);
		expect(remove).toHaveBeenCalledOnce();
		expect(refresh).toHaveBeenCalledOnce();
	});
});

describe("VersionHistoryDialog", () => {
	it("shows the selected File title and loading state when opened", () => {
		const html = renderToString(
			<VersionHistoryDialog
				open
				file={makeFile()}
				onClose={vi.fn()}
				onRestored={vi.fn()}
			/>,
		);
		expect(html).toContain("history.md 的版本历史");
		expect(html).toContain("正在加载版本历史");
	});
});

function makeFile(): DriveFile {
	return {
		id: "file 1",
		name: "history.md",
		path: "/",
		storage_path: "history.md",
		size: 3,
		mime_type: "text/markdown",
		is_dir: false,
		status: "ready",
		chunk_count: 1,
		created_at: "2026-08-12T00:00:00Z",
		updated_at: "2026-08-12T01:00:00Z",
	};
}
