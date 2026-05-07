import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
	createFolder,
	deleteFile,
	downloadUrl,
	listFiles,
	renameFile,
	searchFiles,
} from "../../api/fileApi";
import { AIFloatyBall } from "../../components/AIAssistant/AIFloatyBall";
import { Button, message, Modal } from "../../components/base";
import { FilePreview } from "../../components/FilePreview/FilePreview";
import previewStyles from "../../components/FilePreview/FilePreview.module.css";
import { FileList } from "../../components/FileManager/FileList";
import { MoveDialog } from "../../components/FileManager/MoveDialog";
import { SearchResultList } from "../../components/FileManager/SearchResultList";
import { useChunkedUpload } from "../../hooks/useChunkedUpload";
import { useFileStore } from "../../stores/fileStore";
import type { DriveFile, FileSearchHit } from "../../types";

import styles from "./index.module.css";

const MAX_CRUMB_LEVELS = 3;

export function DrivePage() {
	const { t } = useTranslation();
	const {
		currentPath,
		files,
		selectedFile,
		setCurrentPath,
		setFiles,
		setSelectedFile,
	} = useFileStore();
	const fileInputRef = useRef<HTMLInputElement | null>(null);
	const [error, setError] = useState("");
	const [query, setQuery] = useState("");
	const [searchHits, setSearchHits] = useState<FileSearchHit[] | null>(null);
	const [searching, setSearching] = useState(false);
	const [includeSemantic, setIncludeSemantic] = useState(false);
	const [folderModalOpen, setFolderModalOpen] = useState(false);
	const [folderName, setFolderName] = useState("");
	const [creating, setCreating] = useState(false);
	const [previewFile, setPreviewFile] = useState<DriveFile | null>(null);
	const [fileToDelete, setFileToDelete] = useState<DriveFile | null>(null);
	const [deleting, setDeleting] = useState(false);
	const [renameTarget, setRenameTarget] = useState<DriveFile | null>(null);
	const [newName, setNewName] = useState("");
	const [renaming, setRenaming] = useState(false);
	const [moveTarget, setMoveTarget] = useState<DriveFile | null>(null);
	const { upload } = useChunkedUpload(() => {
		void refresh();
	});
	const searchSerial = useRef(0);

	// Breadcrumbs (computed on each render, fine for this use case)
	function buildCrumbs() {
		const parts = currentPath.split("/").filter(Boolean);
		const all = [{ label: t("drive.rootDir"), path: "/" }].concat(
			parts.map((part: string, i: number) => ({
				label: part,
				path: "/" + parts.slice(0, i + 1).join("/"),
			})),
		) as { label: string; path: string }[];
		if (all.length <= MAX_CRUMB_LEVELS) return all;
		return [
			all[0],
			{ label: "...", path: "" },
			...all.slice(all.length - (MAX_CRUMB_LEVELS - 1)),
		];
	}
	const crumbs = buildCrumbs();

	function getParentPath() {
		if (currentPath === "/") return null;
		const parts = currentPath.split("/").filter(Boolean);
		parts.pop();
		return parts.length === 0 ? "/" : "/" + parts.join("/");
	}
	const parentPath = getParentPath();

	async function refresh(path = currentPath) {
		try {
			const result = await listFiles(path);
			setFiles(result.files);
			setError("");
		} catch (err) {
			setError(err instanceof Error ? err.message : t("drive.loadError"));
		}
	}

	useEffect(() => {
		void refresh(currentPath);
	}, [currentPath]);

	useEffect(() => {
		const text = query.trim();
		if (!text) {
			searchSerial.current += 1;
			setSearchHits(null);
			setSearching(false);
			return;
		}
		const requestID = searchSerial.current + 1;
		searchSerial.current = requestID;
		const timer = window.setTimeout(async () => {
			setSearching(true);
			try {
				const response = await searchFiles({
					query: text,
					path: currentPath,
					semantic: includeSemantic,
					limit: 50,
				});
				if (searchSerial.current !== requestID) return;
				setSearchHits(response.hits);
			} catch (err) {
				if (searchSerial.current !== requestID) return;
				message.error(err instanceof Error ? err.message : "搜索失败");
				setSearchHits([]);
			} finally {
				if (searchSerial.current === requestID) {
					setSearching(false);
				}
			}
		}, 300);
		return () => window.clearTimeout(timer);
	}, [query, currentPath, includeSemantic]);

	function openCreateFolder() {
		setFolderName("");
		setFolderModalOpen(true);
	}

	async function handleCreateFolder() {
		const name = folderName.trim();
		if (!name) return;
		setCreating(true);
		try {
			await createFolder(currentPath, name);
			setFolderModalOpen(false);
			await refresh();
		} finally {
			setCreating(false);
		}
	}

	function onDelete(file: DriveFile) {
		setFileToDelete(file);
	}

	function onRename(file: DriveFile) {
		setRenameTarget(file);
		setNewName(file.name);
	}

	function onDownload(file: DriveFile) {
		if (file.is_dir) return;
		window.open(downloadUrl(file.id), "_self");
	}

	async function handleDeleteConfirm() {
		if (!fileToDelete) return;
		setDeleting(true);
		try {
			await deleteFile(fileToDelete.id);
			setFileToDelete(null);
			setSelectedFile(undefined);
			await refresh();
			message.success(t("drive.deleteSuccess", { name: fileToDelete.name }));
		} catch (err) {
			message.error(
				err instanceof Error ? err.message : t("drive.deleteFailed"),
			);
		} finally {
			setDeleting(false);
		}
	}

	async function handleRenameSubmit() {
		if (!renameTarget) return;
		const trimmed = newName.trim();
		if (!trimmed || trimmed === renameTarget.name || trimmed.includes("/")) return;
		setRenaming(true);
		try {
			await renameFile(renameTarget.id, trimmed);
			setRenameTarget(null);
			setNewName("");
			await refresh();
			message.success(t("drive.renameSuccess"));
		} catch (err) {
			message.error(isHTTPStatus(err, 409) ? t("moveDialog.targetExists") : t("drive.renameFailed"));
		} finally {
			setRenaming(false);
		}
	}

	function openFolder(file: DriveFile) {
		setCurrentPath(
			file.path === "/" ? `/${file.name}` : `${file.path}/${file.name}`,
		);
	}

	function triggerUpload() {
		fileInputRef.current?.click();
	}

	async function handleFiles(files: FileList | null) {
		if (!files || files.length === 0) return;
		const selected = Array.from(files);
		message.info(
			t("drive.filesAddedToTransfer", { count: selected.length }),
		);
		for (const file of selected) {
			void upload(file, currentPath).catch((err) => {
				if (err instanceof Error && err.message === "upload cancelled") return;
				message.error(`${file.name} 上传失败`);
			});
		}
	}

	function handleFileClick(file: DriveFile) {
		setSelectedFile(file);
		if (!file.is_dir) {
			setPreviewFile(file);
		}
	}

	function handleSearchPick(file: DriveFile) {
		setSelectedFile(file);
		if (file.is_dir) {
			openFolder(file);
			return;
		}
		setPreviewFile(file);
	}

	async function handleMoveComplete() {
		setMoveTarget(null);
		setSelectedFile(undefined);
		await refresh();
	}

	function previewTitle(file: DriveFile) {
		const badge =
			file.status === "processing" ? (
				<span
					className={`${previewStyles.statusBadge} ${previewStyles.badgeProcessing}`}
				>
					{t("drive.processing")}
				</span>
			) : file.status === "failed" ? (
				<span
					className={`${previewStyles.statusBadge} ${previewStyles.badgeFailed}`}
				>
					{t("drive.processFailed")}
				</span>
			) : null;

		return (
			<div className={previewStyles.modalTitle}>
				<span className={previewStyles.modalFileName}>{file.name}</span>
				{badge}
				<a
					className={previewStyles.downloadLink}
					href={downloadUrl(file.id)}
					download={file.name}
				>
					{t("common.download")}
				</a>
			</div>
		);
	}

	return (
		<div className={styles.pageWrapper}>
			<div className={styles.header}>
				<div className={styles.titleGroup}>
					<h2>{t("drive.title")}</h2>
					<p>{t("drive.subtitle")}</p>
				</div>
			</div>

			{/* Breadcrumb navigation */}
			<div className={styles.breadcrumbBar}>
				{parentPath && (
					<button
						className={styles.backBtn}
						onClick={() => setCurrentPath(parentPath)}
						title={t("drive.backToParent")}
					>
						<span
							className="material-symbols-outlined"
							style={{ fontSize: 18 }}
						>
							arrow_back
						</span>
					</button>
				)}
				{crumbs.map(
					(crumb: { label: string; path: string }, i: number) => (
						<span key={crumb.path || i} className={styles.crumb}>
							<button
								className={`${styles.crumbBtn} ${i === crumbs.length - 1 ? styles.crumbBtnCurrent : ""}`}
								onClick={() =>
									crumb.path && setCurrentPath(crumb.path)
								}
								disabled={!crumb.path}
							>
								{crumb.label}
							</button>
							{i < crumbs.length - 1 && (
								<span className={styles.crumbSep}>/</span>
							)}
						</span>
					),
				)}
			</div>

			{/* Toolbar */}
			<div className={styles.toolbar}>
				<div className="flex items-center gap-2 flex-1">
					<input
						className={styles.searchInput}
						value={query}
						onChange={(event: any) => setQuery(event.target.value)}
						placeholder={t("drive.searchPlaceholder")}
					/>
					<label className={styles.semanticToggle}>
						<input
							type="checkbox"
							checked={includeSemantic}
							onChange={(event: any) =>
								setIncludeSemantic(Boolean(event.target.checked))
							}
						/>
						<span>{t("drive.semanticSearchHint")}</span>
					</label>
				</div>
				<div className="flex items-center gap-2">
					<Button onClick={openCreateFolder} variant="secondary">
						<span className="material-symbols-outlined text-[14px]">
							create_new_folder
						</span>
						{t("drive.newFolder")}
					</Button>
					<input
						ref={fileInputRef}
						type="file"
						multiple
						hidden
						onChange={(e: any) => {
							void handleFiles(e.target.files);
							e.target.value = "";
						}}
					/>
					<Button variant="primary" onClick={triggerUpload}>
						<span className="material-symbols-outlined text-[14px]">
							upload_file
						</span>
						{t("drive.upload")}
					</Button>
				</div>
			</div>

			{/* File list */}
			<div className={styles.mainGrid}>
				<section className={styles.fileListSection}>
					<div className={styles.fileListInner}>
						{searchHits === null ? (
							<FileList
								files={files}
								selectedId={selectedFile?.id}
								onOpenFolder={openFolder}
								onSelect={handleFileClick}
								onDelete={onDelete}
								onRename={onRename}
								onMove={setMoveTarget}
								onDownload={onDownload}
							/>
						) : (
							<SearchResultList
								hits={searchHits}
								loading={searching}
								onClear={() => setQuery("")}
								onPick={handleSearchPick}
							/>
						)}
					</div>
				</section>
			</div>

			{/* New folder modal */}
			<Modal
				open={folderModalOpen}
				onClose={() => setFolderModalOpen(false)}
				title={t("drive.newFolder")}
				footer={
					<>
						<Button
							variant="secondary"
							onClick={() => setFolderModalOpen(false)}
						>
							{t("common.cancel")}
						</Button>
						<Button
							variant="primary"
							onClick={handleCreateFolder}
							disabled={!folderName.trim() || creating}
							loading={creating}
						>
							{t("common.create")}
						</Button>
					</>
				}
			>
				<div className="flex flex-col gap-2">
					<label className="text-sm font-medium text-warm-gray-500">
						{t("drive.folderName")}
					</label>
					<input
						type="text"
						className="w-full h-10 px-3 border rounded-lg text-sm outline-none transition-colors"
						style={{
							border: "1px solid rgba(0,0,0,0.1)",
							fontFamily: "inherit",
						}}
						value={folderName}
						onChange={(e: any) => setFolderName(e.target.value)}
						onKeyDown={(e: any) => {
							if (e.key === "Enter") handleCreateFolder();
						}}
						placeholder={t("drive.folderNamePlaceholder")}
						autoFocus
					/>
				</div>
			</Modal>

			{/* Rename modal */}
			<Modal
				open={!!renameTarget}
				onClose={() => setRenameTarget(null)}
				title={t("common.rename")}
				footer={
					<>
						<Button
							variant="secondary"
							onClick={() => setRenameTarget(null)}
						>
							{t("common.cancel")}
						</Button>
						<Button
							variant="primary"
							onClick={handleRenameSubmit}
							disabled={
								!newName.trim() ||
								newName.trim() === renameTarget?.name ||
								newName.includes("/") ||
								renaming
							}
							loading={renaming}
						>
							{t("common.save")}
						</Button>
					</>
				}
			>
				<div className="flex flex-col gap-2">
					<label className="text-sm font-medium text-warm-gray-500">
						{t("drive.newName")}
					</label>
					<input
						type="text"
						className="w-full h-10 px-3 border rounded-lg text-sm outline-none transition-colors"
						style={{
							border: "1px solid rgba(0,0,0,0.1)",
							fontFamily: "inherit",
						}}
						value={newName}
						onChange={(e: any) => setNewName(e.target.value)}
						onKeyDown={(e: any) => {
							if (e.key === "Enter") void handleRenameSubmit();
						}}
						placeholder={t("drive.newNamePlaceholder")}
						autoFocus
					/>
					{newName.includes("/") ? (
						<p className="text-xs text-red-600">{t("drive.nameNoSlash")}</p>
					) : null}
				</div>
			</Modal>

			<MoveDialog
				open={!!moveTarget}
				target={moveTarget}
				onClose={() => setMoveTarget(null)}
				onMoved={handleMoveComplete}
			/>

			{/* File preview modal */}
			{previewFile && (
				<Modal
					open={!!previewFile}
					onClose={() => setPreviewFile(null)}
					title={previewTitle(previewFile)}
					width={'80vw'}
          height={'80vh'}
				>
					<FilePreview key={previewFile.id} file={previewFile} />
				</Modal>
			)}

			{/* Delete confirmation modal */}
			<Modal
				open={!!fileToDelete}
				onClose={() => setFileToDelete(null)}
				title={t("drive.confirmDelete")}
				footer={
					<>
						<Button
							variant="secondary"
							onClick={() => setFileToDelete(null)}
						>
							{t("common.cancel")}
						</Button>
						<Button
							variant="danger"
							onClick={handleDeleteConfirm}
							loading={deleting}
						>
							{t("drive.deleteToTrash")}
						</Button>
					</>
				}
			>
				<p className="text-sm text-warm-gray-500">
					{t("drive.deleteConfirmBody", { name: fileToDelete?.name })}
				</p>
			</Modal>

			<AIFloatyBall />
		</div>
	);
}

function isHTTPStatus(err: unknown, status: number) {
	return (
		typeof err === "object" &&
		err !== null &&
		"status" in err &&
		(err as { status?: number }).status === status
	);
}
