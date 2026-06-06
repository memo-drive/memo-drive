import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import {
	createMarkdownFile,
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
import {
	buildDriveCrumbs,
	buildDrivePreviewTitle,
	buildDriveSearchRequest,
	canSubmitDriveFolder,
	canSubmitDriveMarkdown,
	canSubmitDriveRename,
	completeDriveFolderCreate,
	completeDriveMarkdownCreate,
	completeDriveMove,
	confirmDriveDelete,
	driveFolderPayloadName,
	driveMarkdownErrorKey,
	driveMarkdownPayloadName,
	driveParentPath,
	driveRenameErrorKey,
	driveRenamePayloadName,
	pickDriveFile,
	pickSearchResult,
	selectedDriveUploadFiles,
	shouldStartDriveUpload,
	startDriveFolderEntry,
	startDriveDelete,
	startDriveFolderCreate,
	startDriveMarkdownCreate,
	startDriveMove,
	startDriveRename,
	type DriveCrumb,
	type DrivePreviewBadgeTone,
} from "../../workflows/driveWorkflow";
import styles from "./index.module.css";

const MAX_CRUMB_LEVELS = 3;

export function DrivePage() {
	const { t } = useTranslation();
	const navigate = useNavigate();
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
	const [markdownModalOpen, setMarkdownModalOpen] = useState(false);
	const [markdownName, setMarkdownName] = useState("");
	const [creatingMarkdown, setCreatingMarkdown] = useState(false);
	const [creating, setCreating] = useState(false);
	const [previewFile, setPreviewFile] = useState<DriveFile | null>(null);
	const [fileToDelete, setFileToDelete] = useState<DriveFile | null>(null);
	const [deleting, setDeleting] = useState(false);
	const [renameTarget, setRenameTarget] = useState<DriveFile | null>(null);
	const [newName, setNewName] = useState("");
	const [renaming, setRenaming] = useState(false);
	const [moveTarget, setMoveTarget] = useState<DriveFile | null>(null);
	const [enteringFolderId, setEnteringFolderId] = useState<string | null>(null);
	const { upload } = useChunkedUpload(() => {
		void refresh();
	});
	const searchSerial = useRef(0);
	const enteringFolderIdRef = useRef<string | null>(null);
	const enteringPathRef = useRef<string | null>(null);

	const crumbs = buildDriveCrumbs(
		currentPath,
		t("drive.rootDir"),
		MAX_CRUMB_LEVELS,
	);
	const parentPath = driveParentPath(currentPath);

	async function refresh(path = currentPath) {
		try {
			const result = await listFiles(path);
			setFiles(result.files);
			setError("");
		} catch (err) {
			setError(err instanceof Error ? err.message : t("drive.loadError"));
		} finally {
			if (enteringPathRef.current === path) {
				enteringPathRef.current = null;
				enteringFolderIdRef.current = null;
				setEnteringFolderId(null);
			}
		}
	}

	useEffect(() => {
		void refresh(currentPath);
	}, [currentPath]);

	useEffect(() => {
		const request = buildDriveSearchRequest(
			query,
			currentPath,
			includeSemantic,
		);
		if (!request) {
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
				const response = await searchFiles(request);
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
		const draft = startDriveFolderCreate();
		setFolderName(draft.draftName);
		setFolderModalOpen(draft.open);
	}

	function openCreateMarkdown() {
		const draft = startDriveMarkdownCreate();
		setMarkdownName(draft.draftName);
		setMarkdownModalOpen(draft.open);
	}

	async function handleCreateFolder() {
		if (!canSubmitDriveFolder(folderName)) return;
		const name = driveFolderPayloadName(folderName);
		setCreating(true);
		try {
			await createFolder(currentPath, name);
			const next = completeDriveFolderCreate();
			setFolderModalOpen(next.open);
			setFolderName(next.draftName);
			await refresh();
		} finally {
			setCreating(false);
		}
	}

	async function handleCreateMarkdown() {
		if (!canSubmitDriveMarkdown(markdownName)) return;
		const name = driveMarkdownPayloadName(markdownName);
		setCreatingMarkdown(true);
		try {
			const result = await createMarkdownFile(currentPath, name);
			const next = completeDriveMarkdownCreate();
			setMarkdownModalOpen(next.open);
			setMarkdownName(next.draftName);
			await refresh();
			navigate(`/files/${result.file.id}/edit`);
		} catch (err) {
			message.error(err instanceof Error ? err.message : t("drive.createMarkdownFailed"));
		} finally {
			setCreatingMarkdown(false);
		}
	}

	function onDelete(file: DriveFile) {
		setFileToDelete(startDriveDelete(file).target);
	}

	function onRename(file: DriveFile) {
		const draft = startDriveRename(file);
		setRenameTarget(draft.target);
		setNewName(draft.draftName);
	}

	function onDownload(file: DriveFile) {
		if (file.is_dir) return;
		window.open(downloadUrl(file.id), "_self");
	}

	function onEdit(file: DriveFile) {
		if (file.is_dir) return;
		navigate(`/files/${file.id}/edit`);
	}

	async function handleDeleteConfirm() {
		if (!fileToDelete) return;
		setDeleting(true);
		try {
			await deleteFile(fileToDelete.id);
			const next = confirmDriveDelete();
			setFileToDelete(next.deleteTarget);
			setSelectedFile(next.selectedFile);
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
		if (!canSubmitDriveRename(renameTarget, newName)) return;
		const trimmed = driveRenamePayloadName(newName);
		setRenaming(true);
		try {
			await renameFile(renameTarget!.id, trimmed);
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

	const renameErrorKey = driveRenameErrorKey(newName);
	const markdownErrorKey = driveMarkdownErrorKey(markdownName);

	function navigateToPath(path: string) {
		enteringPathRef.current = null;
		enteringFolderIdRef.current = null;
		setEnteringFolderId(null);
		setCurrentPath(path);
	}

	function openFolder(file: DriveFile) {
		const entry = startDriveFolderEntry(file, enteringFolderIdRef.current);
		if (!entry) return;
		enteringFolderIdRef.current = entry.enteringFolderId;
		enteringPathRef.current = entry.nextPath;
		setEnteringFolderId(entry.enteringFolderId);
		setCurrentPath(entry.nextPath);
	}

	function triggerUpload() {
		fileInputRef.current?.click();
	}

	async function handleFiles(files: FileList | null) {
		if (!shouldStartDriveUpload(files)) return;
		const selected = selectedDriveUploadFiles(files);
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
		const pick = pickDriveFile(file);
		setSelectedFile(pick.selectedFile);
		setPreviewFile(pick.previewFile);
	}

	function handleSearchPick(file: DriveFile) {
		const pick = pickSearchResult(file);
		setSelectedFile(pick.selectedFile);
		setPreviewFile(pick.previewFile);
		if (pick.nextPath) navigateToPath(pick.nextPath);
	}

	async function handleMoveComplete() {
		const next = completeDriveMove();
		setMoveTarget(next.moveTarget);
		setSelectedFile(next.selectedFile);
		await refresh();
	}

	function previewTitle(file: DriveFile) {
		const title = buildDrivePreviewTitle(file, downloadUrl(file.id));
		const badgeClass: Record<DrivePreviewBadgeTone, string> = {
			failed: previewStyles.badgeFailed,
			processing: previewStyles.badgeProcessing,
		};
		const badge = title.badge ? (
				<span
					className={`${previewStyles.statusBadge} ${badgeClass[title.badge.tone]}`}
				>
					{t(title.badge.labelKey)}
				</span>
			) : null;

		return (
			<div className={previewStyles.modalTitle}>
				<span className={previewStyles.modalFileName}>{title.fileName}</span>
				{badge}
				<a
					className={previewStyles.downloadLink}
					href={title.downloadHref}
					download={title.downloadFileName}
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
						onClick={() => navigateToPath(parentPath)}
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
					(crumb: DriveCrumb, i: number) => (
						<span key={crumb.path || i} className={styles.crumb}>
							<button
								className={`${styles.crumbBtn} ${i === crumbs.length - 1 ? styles.crumbBtnCurrent : ""}`}
								onClick={() =>
									crumb.path && navigateToPath(crumb.path)
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
					<Button onClick={openCreateMarkdown} variant="secondary">
						<span className="material-symbols-outlined text-[14px]">
							note_add
						</span>
						{t("drive.newMarkdown")}
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
								enteringFolderId={enteringFolderId}
								folderNavigationDisabled={Boolean(enteringFolderId)}
								onOpenFolder={openFolder}
								onSelect={handleFileClick}
								onDelete={onDelete}
								onEdit={onEdit}
								onRename={onRename}
								onMove={(file) => setMoveTarget(startDriveMove(file).target)}
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
							disabled={!canSubmitDriveFolder(folderName) || creating}
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

			<Modal
				open={markdownModalOpen}
				onClose={() => setMarkdownModalOpen(false)}
				title={t("drive.newMarkdown")}
				footer={
					<>
						<Button
							variant="secondary"
							onClick={() => setMarkdownModalOpen(false)}
						>
							{t("common.cancel")}
						</Button>
						<Button
							variant="primary"
							onClick={handleCreateMarkdown}
							disabled={!canSubmitDriveMarkdown(markdownName) || creatingMarkdown}
							loading={creatingMarkdown}
						>
							{t("common.create")}
						</Button>
					</>
				}
			>
				<div className="flex flex-col gap-2">
					<label className="text-sm font-medium text-warm-gray-500">
						{t("drive.markdownName")}
					</label>
					<input
						type="text"
						className="w-full h-10 px-3 border rounded-lg text-sm outline-none transition-colors"
						style={{
							border: "1px solid rgba(0,0,0,0.1)",
							fontFamily: "inherit",
						}}
						value={markdownName}
						onChange={(e: any) => setMarkdownName(e.target.value)}
						onKeyDown={(e: any) => {
							if (e.key === "Enter") void handleCreateMarkdown();
						}}
						placeholder={t("drive.markdownNamePlaceholder")}
						autoFocus
					/>
					{markdownErrorKey ? (
						<p className="text-xs text-red-600">{t(markdownErrorKey)}</p>
					) : null}
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
							disabled={!canSubmitDriveRename(renameTarget, newName) || renaming}
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
					{renameErrorKey ? (
						<p className="text-xs text-red-600">{t(renameErrorKey)}</p>
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
