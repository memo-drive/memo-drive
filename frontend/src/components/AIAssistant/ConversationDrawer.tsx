import { useCallback, useEffect, useState } from "react";
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
      message.success("会话已重命名");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "重命名失败");
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
      message.success("会话已删除");
    } catch (err) {
      message.error(err instanceof Error ? err.message : "删除会话失败");
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
            <h2>历史会话</h2>
          </div>
          <button className={styles.iconBtn} onClick={onClose} aria-label="关闭历史会话">
            <span className="material-symbols-outlined">close</span>
          </button>
        </div>

        <div className={styles.list}>
          {loading ? <p className={styles.empty}>正在加载历史...</p> : null}
          {!loading && conversations.length === 0 ? (
            <p className={styles.empty}>还没有历史会话，问出第一句就会自动保存。</p>
          ) : null}
          {conversations.map((item) => (
            <article
              key={item.id}
              className={`${styles.item} ${
                item.id === conversationId ? styles.active : ""
              }`}
            >
              <button className={styles.itemMain} onClick={() => void handleLoad(item.id)}>
                <span className={styles.title}>{item.title || "新会话"}</span>
                <span className={styles.meta}>
                  {item.mode === "search" ? "语义搜索" : "文件问答"} ·{" "}
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
                  aria-label="重命名会话"
                >
                  <span className="material-symbols-outlined">edit</span>
                </button>
                <button
                  className={styles.iconBtn}
                  disabled={deletingId === item.id}
                  onClick={() => setDeleteTargetId(item.id)}
                  aria-label="删除会话"
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
        title="重命名会话"
        footer={
          <>
            <Button variant="secondary" onClick={() => setRenamingId("")}>
              取消
            </Button>
            <Button onClick={handleRename} disabled={!renameTitle.trim()}>
              保存
            </Button>
          </>
        }
      >
        <input
          className={styles.renameInput}
          value={renameTitle}
          onChange={(event) => setRenameTitle(event.target.value)}
          placeholder="输入会话标题"
          autoFocus
        />
      </Modal>

      <Modal
        open={!!deleteTargetId}
        onClose={() => setDeleteTargetId("")}
        title="删除会话"
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleteTargetId("")}>
              取消
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteConfirm}
              loading={deletingId === deleteTargetId}
            >
              删除
            </Button>
          </>
        }
      >
        <p className="text-sm text-warm-gray-500">
          确定要删除这条会话历史吗？删除后将同时移除会话中的全部消息记录。
        </p>
      </Modal>
    </>
  );
}

function formatDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "刚刚";
  return date.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}
