import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
	deleteFileVersion,
	fileVersionDownloadUrl,
	listFileVersions,
	restoreFileVersion,
} from "../../api/fileApi";
import type { DriveFile, FileVersion, FileVersionSource } from "../../types";
import { Button, message, Modal } from "../base";

interface VersionHistoryListProps {
	file: DriveFile;
	versions: FileVersion[];
	onRestore: (version: FileVersion) => void;
	onDelete: (version: FileVersion) => void;
}

export async function confirmAndDeleteVersion(
	confirmDelete: () => boolean,
	remove: () => Promise<void>,
	refresh: () => Promise<void>,
): Promise<void> {
	if (!confirmDelete()) return;
	await remove();
	await refresh();
}

interface VersionHistoryDialogProps {
	open: boolean;
	file: DriveFile | null;
	onClose: () => void;
	onRestored: (file: DriveFile) => void | Promise<void>;
}

export function VersionHistoryDialog({ open, file, onClose, onRestored }: VersionHistoryDialogProps) {
	const { t } = useTranslation();
	const [versions, setVersions] = useState<FileVersion[]>([]);
	const [loading, setLoading] = useState(true);
	const [busyVersionID, setBusyVersionID] = useState("");

	async function refresh() {
		if (!file) return;
		const result = await listFileVersions(file.id);
		setVersions(result.versions);
	}

	useEffect(() => {
		if (!open || !file) return;
		let active = true;
		setLoading(true);
		listFileVersions(file.id)
			.then((result) => {
				if (active) setVersions(result.versions);
			})
			.catch(() => {
				if (active) message.error(t("fileVersion.loadFailed"));
			})
			.finally(() => {
				if (active) setLoading(false);
			});
		return () => {
			active = false;
		};
	}, [file?.id, open, t]);

	async function handleRestore(version: FileVersion) {
		if (!file || busyVersionID) return;
		setBusyVersionID(version.id);
		try {
			const result = await restoreFileVersion(file.id, version.id);
			await refresh();
			await onRestored(result.file);
			message.success(t("fileVersion.restoreSuccess"));
		} catch {
			message.error(t("fileVersion.restoreFailed"));
		} finally {
			setBusyVersionID("");
		}
	}

	async function handleDelete(version: FileVersion) {
		if (!file || busyVersionID) return;
		setBusyVersionID(version.id);
		try {
			await confirmAndDeleteVersion(
				() => window.confirm(t("fileVersion.deleteConfirm", { number: version.version_no })),
				() => deleteFileVersion(file.id, version.id),
				refresh,
			);
		} catch {
			message.error(t("fileVersion.deleteFailed"));
		} finally {
			setBusyVersionID("");
		}
	}

	return (
		<Modal
			open={open}
			onClose={onClose}
			title={t("fileVersion.title", { name: file?.name ?? "" })}
			width="720px"
			footer={<Button variant="secondary" onClick={onClose}>{t("common.close")}</Button>}
		>
			{loading ? (
				<p role="status" className="py-8 text-center text-sm text-zinc-500">{t("fileVersion.loading")}</p>
			) : file ? (
				<VersionHistoryList file={file} versions={versions} onRestore={handleRestore} onDelete={handleDelete} />
			) : null}
		</Modal>
	);
}

export function VersionHistoryList({ file, versions, onRestore, onDelete }: VersionHistoryListProps) {
	const { t } = useTranslation();
	if (versions.length === 0) {
		return <p className="py-8 text-center text-sm text-zinc-500">{t("fileVersion.empty")}</p>;
	}
	return (
		<ul className="flex flex-col gap-3" aria-label={t("fileVersion.title", { name: file.name })}>
			{versions.map((version) => (
				<li key={version.id} className="rounded-xl border border-zinc-200 p-4">
					<div className="flex items-start justify-between gap-4">
						<div className="min-w-0">
							<strong>{t("fileVersion.versionNumber", { number: version.version_no })}</strong>
							<p className="mt-1 text-xs text-zinc-500">
								{new Date(version.created_at).toLocaleString()} · {formatVersionBytes(version.size)} · {sourceLabel(t, version.source)}
							</p>
							<p className="mt-1 text-xs text-zinc-500">
								{t(version.checksum_status === "recorded" ? "fileVersion.checksumRecorded" : "fileVersion.checksumMissing")}
							</p>
						</div>
						<div className="flex shrink-0 gap-2">
							<a href={fileVersionDownloadUrl(file.id, version.id)} download>{t("common.download")}</a>
							<button type="button" onClick={() => onRestore(version)}>{t("fileVersion.restore")}</button>
							<button type="button" onClick={() => onDelete(version)}>{t("common.delete")}</button>
						</div>
					</div>
				</li>
			))}
		</ul>
	);
}

function formatVersionBytes(bytes: number): string {
	if (bytes < 1024) return `${bytes} B`;
	if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
	return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function sourceLabel(t: (key: string) => string, source: FileVersionSource): string {
	return t(`fileVersion.source.${source}`);
}
