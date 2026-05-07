import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  deleteConversation,
  listConversations,
  renameConversation,
} from "../../api/aiApi";
import { useChatStore } from "../../stores/chatStore";
import { Button, message, Modal } from "../base";
import styles from "./ConversationDrawer.module.css";

interface ConversationDrawerProps {
  open: boolean;
  onClose: () => void;
}

export function ConversationDrawer({ open, onClose }: ConversationDrawerProps) {
  const { t, i18n } = useTranslation();
  const conversations = useChatStore((state) => state.conversations);
  const conversationId = useChatStore((state) => state.conversationId);
  const setConversations = useChatStore((state) => state.setConversations);
  const loadConversation = useChatStore((state) => state.loadConversation);
  const setConversationId = useChatStore((state) => state.setConversationId);
  const [loading, setLoading] = useState(false);
  const [renamingId, setRenamingId] = useState("");
  const [renameTitle, setRenameTitle] = useState("");
  const [deleteTargetId, setDeleteTargetId] = useState("");
  const [deletingId, setDeletingId] = useState("");

  const refresh = useCallback(async () => {
    if (!open) return;
    setLoading(true);
    try {
      const response = await listConversations();
      setConversations(response.conversations ?? []);
    } catch (err) {
      message.error(err instanceof Error ? err.message : "加载历史会话失败");
    } finally {
      setLoading(false);
    }
  }, [open, setConversations]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function handleLoad(id: string) {
    try {
      await loadConversation(id);
      onClose();
    } catch (err) {
      message.error(err instanceof Error ? err.message : "打开会话失败");
    }
  }

  async function handleRename() {
    const title = renameTitle.trim();
    if (!renamingId || !title) return;
    try {
      await renameConversation(renamingId, title);
      setRenamingId("");
      setRenameTitle("");
      await refresh();
      message.success(t("ai.historyRenameSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("ai.historyRenameFailed"));
    }
  }

  async function handleDeleteConfirm() {
    if (!deleteTargetId) return;
    setDeletingId(deleteTargetId);
    try {
      await deleteConversation(deleteTargetId);
      if (conversationId === deleteTargetId) {
        setConversationId(undefined);
      }
      setDeleteTargetId("");
      await refresh();
      message.success(t("ai.historyDeleteSuccess"));
    } catch (err) {
      message.error(err instanceof Error ? err.message : t("ai.historyDeleteFailed"));
    } finally {
      setDeletingId("");
    }
  }

  return (
    <>
      <div
        className={`${styles.backdrop} ${open ? styles.open : ""}`}
        onClick={onClose}
        aria-hidden={!open}
      />
      <aside className={`${styles.drawer} ${open ? styles.open : ""}`} aria-hidden={!open}>
        <div className={styles.header}>
          <div>
            <p>History</p>
            <h2>{t("ai.historyTitle")}</h2>
          </div>
          <button className={styles.iconBtn} onClick={onClose} aria-label={t("common.close")}>
            <span className="material-symbols-outlined">close</span>
          </button>
        </div>

        <div className={styles.list}>
          {loading ? <p className={styles.empty}>{t("ai.historyLoading")}</p> : null}
          {!loading && conversations.length === 0 ? (
            <p className={styles.empty}>{t("ai.historyEmpty")}</p>
          ) : null}
          {conversations.map((item) => (
            <article
              key={item.id}
              className={`${styles.item} ${
                item.id === conversationId ? styles.active : ""
              }`}
            >
              <button className={styles.itemMain} onClick={() => void handleLoad(item.id)}>
                <span className={styles.title}>{item.title || t("ai.historyNew")}</span>
                <span className={styles.meta}>
                  {item.mode === "search" ? t("ai.historySearch") : t("ai.historyRag")} ·{" "}
                  {formatDate(item.updated_at)}
                </span>
              </button>
              <div className={styles.actions}>
                <button
                  className={styles.iconBtn}
                  onClick={() => {
                    setRenamingId(item.id);
                    setRenameTitle(item.title || "");
                  }}
                  aria-label={t("ai.historyRename")}
                >
                  <span className="material-symbols-outlined">edit</span>
                </button>
                <button
                  className={styles.iconBtn}
                  disabled={deletingId === item.id}
                  onClick={() => setDeleteTargetId(item.id)}
                  aria-label={t("ai.historyDelete")}
                >
                  <span className="material-symbols-outlined">delete</span>
                </button>
              </div>
            </article>
          ))}
        </div>
      </aside>

      <Modal
        open={!!renamingId}
        onClose={() => setRenamingId("")}
        title={t("ai.historyRename")}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRenamingId("")}>
              {t("common.cancel")}
            </Button>
            <Button onClick={handleRename} disabled={!renameTitle.trim()}>
              {t("common.save")}
            </Button>
          </>
        }
      >
        <input
          className={styles.renameInput}
          value={renameTitle}
          onChange={(event) => setRenameTitle(event.target.value)}
          placeholder={t("ai.historyRenamePlaceholder")}
          autoFocus
        />
      </Modal>

      <Modal
        open={!!deleteTargetId}
        onClose={() => setDeleteTargetId("")}
        title={t("ai.historyDelete")}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleteTargetId("")}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteConfirm}
              loading={deletingId === deleteTargetId}
            >
              {t("ai.historyDelete")}
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          {t("ai.historyDeleteConfirm")}
        </p>
      </Modal>
    </>
  );

  function formatDate(value: string) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return t("ai.historyJustNow");
    return date.toLocaleString(i18n.language, {
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
    });
  }
}
