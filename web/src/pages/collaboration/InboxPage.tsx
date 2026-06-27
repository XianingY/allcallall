import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Check, ChevronLeft, FileAudio, MessageSquarePlus, Plus, Search, Send, StickyNote, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import { createConversation, createNote, getConversation, listConversations, listMessages, listNotes, markConversationRead, sendMessage, updateConversation } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

const formatTime = (value?: string | null) => value ? new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)) : "";

export function InboxPage() {
  const { conversationId } = useParams();
  const selectedId = Number(conversationId) || null;
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { activeOrganization } = useOrganization();
  const orgId = activeOrganization?.id;
  const [filter, setFilter] = useState("");
  const [composer, setComposer] = useState("");
  const [note, setNote] = useState("");
  const [creating, setCreating] = useState(false);

  const conversations = useQuery({ queryKey: ["organizations", orgId, "conversations", filter], queryFn: () => listConversations(filter), enabled: Boolean(orgId) });
  const detail = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId], queryFn: () => getConversation(selectedId!), enabled: Boolean(orgId && selectedId) });
  const messages = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId, "messages"], queryFn: () => listMessages(selectedId!), enabled: Boolean(orgId && selectedId) });
  const notes = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId, "notes"], queryFn: () => listNotes(selectedId!), enabled: Boolean(orgId && selectedId) });

  useEffect(() => { if (selectedId) void markConversationRead(selectedId).then(() => queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] })); }, [selectedId, orgId, queryClient]);

  const send = useMutation({ mutationFn: () => sendMessage(selectedId!, composer), onSuccess: (message) => { queryClient.setQueryData(["organizations", orgId, "conversations", selectedId, "messages"], (items: typeof messages.data = []) => [...(items ?? []), message]); setComposer(""); } });
  const addNote = useMutation({ mutationFn: () => createNote(selectedId!, note), onSuccess: () => { setNote(""); void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations", selectedId, "notes"] }); void detail.refetch(); } });
  const update = useMutation({ mutationFn: (input: { status?: string; priority?: string }) => updateConversation(selectedId!, input), onSuccess: () => { void detail.refetch(); void conversations.refetch(); } });

  return <div className={`inbox-layout ${selectedId ? "inbox-selected" : ""}`}>
    <aside className="conversation-list">
      <header className="workspace-pane-header"><div><span className="eyebrow">Workspace</span><h1>Inbox</h1></div><NewConversationDialog open={creating} onOpenChange={setCreating} orgId={orgId} onCreated={(id) => navigate(`/conversations/${id}`)} /></header>
      <div className="search-field"><Search size={16} /><input aria-label="搜索会话" placeholder="搜索会话" value={filter} onChange={(event) => setFilter(event.target.value)} /></div>
      <div className="filter-tabs"><button className={!filter ? "active" : ""} onClick={() => setFilter("")}>全部</button><button className={filter === "unread" ? "active" : ""} onClick={() => setFilter("unread")}>未读</button><button className={filter === "open" ? "active" : ""} onClick={() => setFilter("open")}>处理中</button></div>
      {conversations.isLoading ? <PageLoading /> : conversations.isError ? <PageError error={conversations.error} /> : <div className="conversation-items">{conversations.data?.map((item) => <Link key={item.id} to={`/conversations/${item.id}`} className={`conversation-item ${selectedId === item.id ? "conversation-item-active" : ""}`}><div className="conversation-avatar">{item.title.slice(0, 1).toUpperCase()}</div><div className="conversation-copy"><div><strong>{item.title}</strong><time>{formatTime(item.last_message_at)}</time></div><p>{item.last_message_preview || item.topic || "暂无消息"}</p><span>{item.priority}</span></div>{item.unread_count > 0 && <b className="unread-count">{item.unread_count}</b>}</Link>)}</div>}
    </aside>
    <main className="message-pane">
      {!selectedId ? <div className="pane-empty"><MessageSquarePlus size={28} /><strong>选择一个会话</strong><span>消息、备注和 Agent 上下文会在这里显示</span></div> : detail.isLoading ? <PageLoading /> : detail.isError ? <PageError error={detail.error} /> : <>
        <header className="workspace-pane-header"><button className="icon-button mobile-only" aria-label="返回会话列表" onClick={() => navigate("/inbox")}><ChevronLeft size={20} /></button><div className="min-w-0"><h2>{detail.data?.conversation.title}</h2><p>{detail.data?.conversation.topic || "无主题"}</p></div><span className={`status-dot status-${detail.data?.conversation.status}`} /> </header>
        <div className="message-stream">{messages.isLoading ? <PageLoading /> : messages.data?.length ? messages.data.map((message) => <article className="message-bubble" key={message.id}><header><strong>{message.sender_display_name || message.sender_email}</strong><time>{formatTime(message.created_at)}</time></header><p>{message.body}</p></article>) : <div className="pane-empty"><span>还没有消息</span></div>}</div>
        <form className="message-composer" onSubmit={(event) => { event.preventDefault(); if (composer.trim()) send.mutate(); }}><textarea aria-label="输入消息" placeholder="输入消息" rows={2} value={composer} onChange={(event) => setComposer(event.target.value)} /><button className="icon-button composer-send" aria-label="发送消息" disabled={!composer.trim() || send.isPending}><Send size={18} /></button></form>
      </>}
    </main>
    <aside className="context-pane">
      {!selectedId || !detail.data ? <div className="pane-empty"><Bot size={24} /><span>业务上下文</span></div> : <div className="context-scroll">
        <section className="context-section"><h3>会话状态</h3><label>状态<select className="field" value={detail.data.conversation.status} onChange={(event) => update.mutate({ status: event.target.value })}><option value="open">处理中</option><option value="pending">待处理</option><option value="closed">已关闭</option></select></label><label>优先级<select className="field" value={detail.data.conversation.priority} onChange={(event) => update.mutate({ priority: event.target.value })}><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></label></section>
        <section className="context-section"><h3><Bot size={16} />Agent 上下文</h3><Metric label="会议转写" value={String(detail.data.workspace.agent_context.meeting_transcript_segment_count ?? 0)} /><Metric label="知识来源" value={String(detail.data.workspace.agent_context.knowledge_source_count ?? 0)} /><Metric label="待审批" value={String(detail.data.workspace.agent_context.pending_approval_count ?? 0)} />{detail.data.workspace.agent_context.meeting_transcription_status && <span className="context-status">转写 {detail.data.workspace.agent_context.meeting_transcription_status}</span>}<Link className="button-secondary w-full" to={`/agent-lab?conversationId=${selectedId}`}>打开 Agent Lab</Link></section>
        {detail.data.conversation.latest_recording_id && <section className="context-section"><h3><FileAudio size={16} />最新录音</h3><Link to={`/recordings/${detail.data.conversation.latest_recording_id}`} className="button-secondary w-full">查看转写</Link></section>}
        <section className="context-section"><h3><StickyNote size={16} />内部备注</h3><div className="notes-list">{notes.data?.map((item) => <article key={item.id}><p>{item.body}</p><small>{item.author_display_name} · {formatTime(item.created_at)}</small></article>)}</div><textarea className="field" rows={3} placeholder="仅团队可见" value={note} onChange={(event) => setNote(event.target.value)} /><button className="button-secondary w-full" disabled={!note.trim()} onClick={() => addNote.mutate()}><Check size={16} />添加备注</button></section>
      </div>}
    </aside>
  </div>;
}

function Metric({ label, value }: { label: string; value: string }) { return <div className="context-metric"><span>{label}</span><strong>{value}</strong></div>; }

function NewConversationDialog({ open, onOpenChange, orgId, onCreated }: { open: boolean; onOpenChange(open: boolean): void; orgId?: number; onCreated(id: number): void }) {
  const queryClient = useQueryClient(); const [title, setTitle] = useState(""); const [topic, setTopic] = useState("");
  const mutation = useMutation({ mutationFn: () => createConversation({ type: "group", title, topic }), onSuccess: (item) => { void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] }); onOpenChange(false); setTitle(""); setTopic(""); onCreated(item.id); } });
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Trigger asChild><button className="icon-button" aria-label="新建会话"><Plus size={19} /></button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>新建会话</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>创建团队协作上下文，之后可关联联系人、会议和 Agent。</Dialog.Description><FormError error={mutation.error} /><div className="form-stack"><label>标题<input className="field" value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>主题<textarea className="field" value={topic} onChange={(event) => setTopic(event.target.value)} /></label><button className="button-primary" disabled={!title.trim()} onClick={() => mutation.mutate()}>创建会话</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>;
}
