import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Edit3, MoreHorizontal, Paperclip, Pin, Plus, Reply, Trash2, X } from "lucide-react";
import { useState } from "react";

import { createConversation, type Message } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { formatBytes, formatTime } from "@/pages/collaboration/InboxFormat";

export function MessageBubble({ message, currentUserId, onReply, onEdit, onAction }: { message: Message; currentUserId?: number; onReply(message: Message): void; onEdit(message: Message): void; onAction(action: "delete" | "pin" | "unpin" | "react", message: Message, emoji?: string): void }) {
  const mine = message.sender_id === currentUserId;
  return <article id={`message-${message.id}`} className={`message-bubble ${message.deleted_at ? "message-deleted" : ""}`}>
    <header><div><strong>{message.sender_display_name || message.sender_email}</strong>{message.edited_at && <span>已编辑</span>}</div><div className="message-actions"><time>{formatTime(message.created_at)}</time><button className="icon-button" title="回复" onClick={() => onReply(message)}><Reply size={14} /></button>{mine && !message.deleted_at && <button className="icon-button" title="编辑" onClick={() => onEdit(message)}><Edit3 size={14} /></button>}<button className="icon-button" title={message.pinned ? "取消置顶" : "置顶"} onClick={() => onAction(message.pinned ? "unpin" : "pin", message)}><Pin size={14} /></button><button className="icon-button" title="赞同" onClick={() => onAction("react", message, "+1")}><MoreHorizontal size={14} /></button>{!message.deleted_at && <button className="icon-button text-danger" title="撤回" onClick={() => onAction("delete", message)}><Trash2 size={14} /></button>}</div></header>
    {message.reply_to && <div className="reply-preview"><Reply size={13} /><span>{message.reply_to.deleted ? "原消息已撤回" : `${message.reply_to.sender_display_name || message.reply_to.sender_email}: ${message.reply_to.body}`}</span></div>}
    <p>{message.deleted_at ? "该消息已撤回" : message.body}</p>
    {message.attachments?.length ? <div className="attachment-list">{message.attachments.map((item) => <a key={item.id} href={item.download_url} target="_blank" rel="noreferrer"><Paperclip size={13} /><span>{item.file_name}</span><small>{formatBytes(item.file_size)}</small></a>)}</div> : null}
    {message.reactions?.length ? <div className="reaction-row">{message.reactions.map((item) => <button key={item.emoji}>{item.emoji} {item.count}</button>)}</div> : null}
  </article>;
}

export function Metric({ label, value }: { label: string; value: string }) {
  return <div className="context-metric"><span>{label}</span><strong>{value}</strong></div>;
}

export function NewConversationDialog({ open, onOpenChange, orgId, onCreated }: { open: boolean; onOpenChange(open: boolean): void; orgId?: number; onCreated(id: number): void }) {
  const queryClient = useQueryClient();
  const [title, setTitle] = useState("");
  const [topic, setTopic] = useState("");
  const mutation = useMutation({ mutationFn: () => createConversation({ type: "channel", title, topic }), onSuccess: (item) => { void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] }); onOpenChange(false); setTitle(""); setTopic(""); onCreated(item.id); } });
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Trigger asChild><button className="icon-button" aria-label="新建会话"><Plus size={19} /></button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>新建会话</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>创建团队协作上下文，之后可关联联系人、会议和 Agent。</Dialog.Description><FormError error={mutation.error} /><div className="form-stack"><label>标题<input className="field" value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>主题<textarea className="field" value={topic} onChange={(event) => setTopic(event.target.value)} /></label><button className="button-primary" disabled={!title.trim()} onClick={() => mutation.mutate()}>创建会话</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>;
}
