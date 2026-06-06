import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import CodeMirror from "@uiw/react-codemirror";
import { markdown } from "@codemirror/lang-markdown";
import { keymap } from "@codemirror/view";
import {
	createMarkdownFile,
	getMarkdownContent,
	saveMarkdownContent,
} from "../../api/fileApi";
import { HttpError } from "../../api/HttpClient";
import { Button, message, Modal } from "../../components/base";
import type { DriveFile } from "../../types";
import styles from "./index.module.css";

const MAX_MARKDOWN_BYTES = 1 * 1024 * 1024;

type PendingConfirm = "leave" | "reload" | null;

interface DraftPayload {
	content: string;
	savedAt: number;
}

function draftKey(fileID: string, updatedAt: string) {
	return `memodrive.markdown.draft.${fileID}.${updatedAt}`;
}

function readDraft(key: string): DraftPayload | null {
	try {
		const raw = localStorage.getItem(key);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as DraftPayload;
		if (typeof parsed.content !== "string" || typeof parsed.savedAt !== "number") {
			return null;
		}
		return parsed;
	} catch {
		return null;
	}
}

function writeDraft(key: string, content: string) {
	localStorage.setItem(key, JSON.stringify({ content, savedAt: Date.now() }));
}

function markdownBytes(content: string) {
	return new Blob([content]).size;
}

function markdownPath(file: DriveFile | null) {
	if (!file) return "";
	return file.path === "/" ? `/${file.name}` : `${file.path}/${file.name}`;
}

function defaultCopyName(file: DriveFile | null) {
	if (!file) return "copy.md";
	const dot = file.name.lastIndexOf(".");
	const base = dot > 0 ? file.name.slice(0, dot) : file.name;
	return `${base}-copy.md`;
}

export function MarkdownEditorPage() {
	const { t } = useTranslation();
	const navigate = useNavigate();
	const { id = "" } = useParams();
	const [file, setFile] = useState<DriveFile | null>(null);
	const [content, setContent] = useState("");
	const [baseContent, setBaseContent] = useState("");
	const [baseUpdatedAt, setBaseUpdatedAt] = useState("");
	const [draftStorageKey, setDraftStorageKey] = useState("");
	const [loading, setLoading] = useState(true);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState("");
	const [draftPrompt, setDraftPrompt] = useState<DraftPayload | null>(null);
	const [conflicted, setConflicted] = useState(false);
	const [saveAsOpen, setSaveAsOpen] = useState(false);
	const [saveAsName, setSaveAsName] = useState("");
	const [pendingConfirm, setPendingConfirm] = useState<PendingConfirm>(null);

	const dirty = content !== baseContent;
	const bytes = useMemo(() => markdownBytes(content), [content]);
	const lineCount = useMemo(() => (content ? content.split(/\r?\n/).length : 1), [content]);
	const wordCount = useMemo(() => {
		const matches = content.trim().match(/[\p{L}\p{N}_-]+/gu);
		return matches?.length ?? 0;
	}, [content]);

	const loadContent = useCallback(async () => {
		if (!id) return;
		setLoading(true);
		setError("");
		setDraftPrompt(null);
		setConflicted(false);
		try {
			const response = await getMarkdownContent(id);
			setFile(response.file);
			setContent(response.content);
			setBaseContent(response.content);
			setBaseUpdatedAt(response.updated_at);
			const key = draftKey(response.file.id, response.updated_at);
			setDraftStorageKey(key);
			const draft = readDraft(key);
			if (draft && draft.content !== response.content) {
				setDraftPrompt(draft);
			}
		} catch (err) {
			setError(err instanceof Error ? err.message : t("markdownEditor.loadFailed"));
		} finally {
			setLoading(false);
		}
	}, [id, t]);

	useEffect(() => {
		void loadContent();
	}, [loadContent]);

	useEffect(() => {
		if (!draftStorageKey || !dirty) return;
		writeDraft(draftStorageKey, content);
	}, [content, dirty, draftStorageKey]);

	const handleSave = useCallback(async () => {
		if (!id || saving || !baseUpdatedAt) return true;
		if (bytes > MAX_MARKDOWN_BYTES) {
			message.error(t("markdownEditor.tooLarge"));
			return false;
		}
		setSaving(true);
		setConflicted(false);
		try {
			const response = await saveMarkdownContent(id, content, baseUpdatedAt);
			if (draftStorageKey) localStorage.removeItem(draftStorageKey);
			const nextKey = draftKey(response.file.id, response.updated_at);
			setFile(response.file);
			setBaseContent(response.content);
			setContent(response.content);
			setBaseUpdatedAt(response.updated_at);
			setDraftStorageKey(nextKey);
			message.success(t("markdownEditor.savedIndexing"));
			return true;
		} catch (err) {
			if (err instanceof HttpError && err.status === 409) {
				setConflicted(true);
				message.error(t("markdownEditor.conflictTitle"));
				return false;
			}
			message.error(err instanceof Error ? err.message : t("markdownEditor.saveFailed"));
			return false;
		} finally {
			setSaving(false);
		}
	}, [baseUpdatedAt, bytes, content, draftStorageKey, id, saving, t]);

	const extensions = useMemo(
		() => [
			markdown(),
			keymap.of([
				{
					key: "Mod-s",
					run: () => {
						void handleSave();
						return true;
					},
				},
			]),
		],
		[handleSave],
	);

	function goBack() {
		if (dirty) {
			setPendingConfirm("leave");
			return;
		}
		navigate("/");
	}

	function restoreDraft() {
		if (!draftPrompt) return;
		setContent(draftPrompt.content);
		setDraftPrompt(null);
	}

	function discardDraft() {
		if (draftStorageKey) localStorage.removeItem(draftStorageKey);
		setDraftPrompt(null);
	}

	async function reloadServerVersion() {
		if (dirty) {
			setPendingConfirm("reload");
			return;
		}
		await loadContent();
	}

	async function confirmPendingAction() {
		const action = pendingConfirm;
		setPendingConfirm(null);
		if (action === "leave") {
			navigate("/");
			return;
		}
		if (action === "reload") {
			await loadContent();
		}
	}

	async function saveAsMarkdown() {
		const name = saveAsName.trim();
		if (!name || !file) return;
		setSaving(true);
		try {
			const created = await createMarkdownFile(file.path, name);
			const createdContent = await getMarkdownContent(created.file.id);
			await saveMarkdownContent(created.file.id, content, createdContent.updated_at);
			if (draftStorageKey) localStorage.removeItem(draftStorageKey);
			message.success(t("markdownEditor.savedAs"));
			navigate(`/files/${created.file.id}/edit`, { replace: true });
		} catch (err) {
			message.error(err instanceof Error ? err.message : t("markdownEditor.saveFailed"));
		} finally {
			setSaving(false);
			setSaveAsOpen(false);
		}
	}

	if (loading) {
		return (
			<div className={styles.centerState}>
				<span className="material-symbols-outlined">hourglass_top</span>
				{t("markdownEditor.loading")}
			</div>
		);
	}

	if (error) {
		return (
			<div className={styles.centerState}>
				<span className="material-symbols-outlined">error</span>
				<p>{error}</p>
				<Button variant="secondary" onClick={() => void loadContent()}>
					{t("common.retry")}
				</Button>
			</div>
		);
	}

	return (
		<div className={styles.page}>
			<header className={styles.toolbar}>
				<div className={styles.leftTools}>
					<button className={styles.iconButton} onClick={goBack} title={t("common.back")}>
						<span className="material-symbols-outlined">arrow_back</span>
					</button>
					<div className={styles.fileMeta}>
						<strong>{file?.name}</strong>
						<span>{markdownPath(file)}</span>
					</div>
				</div>
				<div className={styles.rightTools}>
					<span className={dirty ? styles.statusUnsaved : styles.statusSaved}>
						{dirty ? t("markdownEditor.unsaved") : t("markdownEditor.saved")}
					</span>
					<Button
						variant="primary"
						onClick={() => void handleSave()}
						disabled={!dirty || saving || bytes > MAX_MARKDOWN_BYTES}
						loading={saving}
					>
						<span className="material-symbols-outlined text-[14px]">save</span>
						{t("common.save")}
					</Button>
				</div>
			</header>

			{draftPrompt ? (
				<div className={styles.notice}>
					<span>{t("markdownEditor.draftFound")}</span>
					<button onClick={restoreDraft}>{t("markdownEditor.restoreDraft")}</button>
					<button onClick={discardDraft}>{t("markdownEditor.discardDraft")}</button>
				</div>
			) : null}

			{conflicted ? (
				<div className={styles.noticeDanger}>
					<span>{t("markdownEditor.conflictBody")}</span>
					<button onClick={() => void reloadServerVersion()}>{t("markdownEditor.reloadServer")}</button>
					<button
						onClick={() => {
							setSaveAsName(defaultCopyName(file));
							setSaveAsOpen(true);
						}}
					>
						{t("markdownEditor.saveAs")}
					</button>
				</div>
			) : null}

			<main className={styles.editorSurface}>
				<CodeMirror
					value={content}
					height="100%"
					extensions={extensions}
					basicSetup={{
						lineNumbers: true,
						foldGutter: false,
						highlightActiveLine: true,
						highlightActiveLineGutter: true,
					}}
					onChange={setContent}
					className={styles.codeMirror}
				/>
			</main>

			<footer className={styles.footer}>
				<span>{t("markdownEditor.lines", { count: lineCount })}</span>
				<span>{t("markdownEditor.words", { count: wordCount })}</span>
				<span className={bytes > MAX_MARKDOWN_BYTES ? styles.byteLimitDanger : ""}>
					{t("markdownEditor.bytes", { count: bytes })}
				</span>
			</footer>

			<Modal
				open={saveAsOpen}
				onClose={() => setSaveAsOpen(false)}
				title={t("markdownEditor.saveAsTitle")}
				footer={
					<>
						<Button variant="secondary" onClick={() => setSaveAsOpen(false)}>
							{t("common.cancel")}
						</Button>
						<Button
							variant="primary"
							onClick={saveAsMarkdown}
							disabled={!saveAsName.trim() || saving}
							loading={saving}
						>
							{t("markdownEditor.saveAs")}
						</Button>
					</>
				}
			>
				<div className={styles.modalField}>
					<label>{t("markdownEditor.fileName")}</label>
					<input
						value={saveAsName}
						onChange={(event: any) => setSaveAsName(event.target.value)}
						onKeyDown={(event: any) => {
							if (event.key === "Enter") void saveAsMarkdown();
						}}
						autoFocus
					/>
				</div>
			</Modal>

			<Modal
				open={pendingConfirm !== null}
				onClose={() => setPendingConfirm(null)}
				title={
					pendingConfirm === "reload"
						? t("markdownEditor.reloadTitle")
						: t("markdownEditor.leaveTitle")
				}
				footer={
					<>
						<Button variant="secondary" onClick={() => setPendingConfirm(null)}>
							{t("common.cancel")}
						</Button>
						<Button variant="primary" onClick={() => void confirmPendingAction()}>
							{pendingConfirm === "reload"
								? t("markdownEditor.reloadServer")
								: t("markdownEditor.leave")}
						</Button>
					</>
				}
			>
				<p className={styles.confirmText}>
					{pendingConfirm === "reload"
						? t("markdownEditor.reloadConfirm")
						: t("markdownEditor.leaveConfirm")}
				</p>
			</Modal>
		</div>
	);
}
