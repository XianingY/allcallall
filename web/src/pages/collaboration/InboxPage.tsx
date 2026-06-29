import * as Dialog from "@radix-ui/react-dialog";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Bot,
  Check,
  ChevronLeft,
  Edit3,
  FileAudio,
  MessageSquarePlus,
  MoreHorizontal,
  Paperclip,
  Pin,
  Plus,
  Reply,
  Search,
  Send,
  StickyNote,
  Trash2,
  Video,
  X,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";

import {
  addReaction,
  createConversation,
  createNote,
  deleteMessage,
  editMessage,
  getConversation,
  listConversations,
  listMessages,
  listNotes,
  listPinnedMessages,
  markConversationRead,
  pinMessage,
  sendMessage,
  sendTyping,
  type Attachment,
  type Message,
  unpinMessage,
  updateConversation,
  uploadAttachment,
} from "@/api/collaboration";
import { createRoom } from "@/api/meetings";
import { useAuth } from "@/auth/AuthProvider";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

const formatTime = (value?: string | null) => value ? new Intl.DateTimeFormat(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" }).format(new Date(value)) : "";
const messageQueryKey = (orgId?: number, conversationId?: number | null) => ["organizations", orgId, "conversations", conversationId, "messages"] as const;

export function InboxPage() {
  const { conversationId } = useParams();
  const selectedId = Number(conversationId) || null;
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const { activeOrganization } = useOrganization();
  const orgId = activeOrganization?.id;
  const [filter, setFilter] = useState("");
  const [composer, setComposer] = useState("");
  const [note, setNote] = useState("");
  const [creating, setCreating] = useState(false);
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [editing, setEditing] = useState<Message | null>(null);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [typingUsers, setTypingUsers] = useState<Record<number, number>>({});
  const typingTimer = useRef<number>();
  const fileInput = useRef<HTMLInputElement>(null);

  const conversations = useQuery({ queryKey: ["organizations", orgId, "conversations", filter], queryFn: () => listConversations(filter), enabled: Boolean(orgId) });
  const detail = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId], queryFn: () => getConversation(selectedId!), enabled: Boolean(orgId && selectedId) });
  const messages = useInfiniteQuery({
    queryKey: messageQueryKey(orgId, selectedId),
    queryFn: ({ pageParam }) => listMessages(selectedId!, { beforeId: pageParam, limit: 50 }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (page) => page.has_more_prev ? page.next_before_id ?? undefined : undefined,
    enabled: Boolean(orgId && selectedId),
  });
  const pins = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId, "pins"], queryFn: () => listPinnedMessages(selectedId!), enabled: Boolean(orgId && selectedId) });
  const notes = useQuery({ queryKey: ["organizations", orgId, "conversations", selectedId, "notes"], queryFn: () => listNotes(selectedId!), enabled: Boolean(orgId && selectedId) });

  const messageItems = useMemo(() => {
    const pages = messages.data?.pages ?? [];
    return pages.slice().reverse().flatMap((page) => page.messages);
  }, [messages.data?.pages]);
  const activeTypingUsers = useMemo(() => Object.entries(typingUsers).filter(([id, until]) => Number(id) !== user?.id && until > Date.now()).map(([id]) => Number(id)), [typingUsers, user?.id]);

  useEffect(() => {
    queueMicrotask(() => {
      setReplyTo(null);
      setEditing(null);
      setAttachments([]);
    });
    if (selectedId) void markConversationRead(selectedId).then(() => queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] }));
  }, [selectedId, orgId, queryClient]);

  useEffect(() => {
    const listener = (event: Event) => {
      const detailEvent = event as CustomEvent<{ event: string; payload: { conversation_id?: number; user_id?: number; typing?: boolean } }>;
      if (!selectedId || detailEvent.detail.payload.conversation_id !== selectedId || !detailEvent.detail.payload.user_id) return;
      if (!detailEvent.detail.event.startsWith("typing.")) return;
      setTypingUsers((current) => ({ ...current, [detailEvent.detail.payload.user_id!]: detailEvent.detail.payload.typing ? Date.now() + 3000 : 0 }));
    };
    window.addEventListener("allcallall:chat-event", listener);
    return () => window.removeEventListener("allcallall:chat-event", listener);
  }, [selectedId]);

  const refreshMessages = () => {
    void queryClient.invalidateQueries({ queryKey: messageQueryKey(orgId, selectedId) });
    void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations", selectedId, "pins"] });
    void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] });
  };
  const send = useMutation({
    mutationFn: () => {
      if (!selectedId) throw new Error("missing conversation");
      if (editing) return editMessage(selectedId, editing.id, composer);
      return sendMessage(selectedId, { body: composer, reply_to_message_id: replyTo?.id, attachment_ids: attachments.map((item) => item.id) });
    },
    onSuccess: () => {
      setComposer("");
      setReplyTo(null);
      setEditing(null);
      setAttachments([]);
      refreshMessages();
    },
  });
  const upload = useMutation({ mutationFn: (file: File) => uploadAttachment(selectedId!, file), onSuccess: (item) => setAttachments((items) => [...items, item]) });
  const addNote = useMutation({ mutationFn: () => createNote(selectedId!, note), onSuccess: () => { setNote(""); void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations", selectedId, "notes"] }); void detail.refetch(); } });
  const update = useMutation({ mutationFn: (input: { status?: string; priority?: string }) => updateConversation(selectedId!, input), onSuccess: () => { void detail.refetch(); void conversations.refetch(); } });
  const messageAction = useMutation({
    mutationFn: async (input: { action: "delete" | "pin" | "unpin" | "react"; message: Message; emoji?: string }) => {
      if (!selectedId) throw new Error("missing conversation");
      if (input.action === "delete") return deleteMessage(selectedId, input.message.id);
      if (input.action === "pin") return pinMessage(selectedId, input.message.id);
      if (input.action === "unpin") return unpinMessage(selectedId, input.message.id);
      return addReaction(selectedId, input.message.id, input.emoji ?? "+1");
    },
    onSuccess: refreshMessages,
  });
  const startMeeting = useMutation({
    mutationFn: () => createRoom({ title: detail.data?.conversation.title || "Team Meeting", conversation_id: selectedId ?? undefined }),
    onSuccess: (room) => { void detail.refetch(); navigate(`/meetings/${room.room.id}/preflight`); },
  });

  const onComposerChange = (value: string) => {
    setComposer(value);
    if (!selectedId) return;
    window.clearTimeout(typingTimer.current);
    void sendTyping(selectedId, true).catch(() => undefined);
    typingTimer.current = window.setTimeout(() => { void sendTyping(selectedId, false).catch(() => undefined); }, 1200);
  };

  return <div className={`inbox-layout ${selectedId ? "inbox-selected" : ""}`}>
    <aside className="conversation-list">
      <header className="workspace-pane-header"><div><span className="eyebrow">Workspace</span><h1>Inbox</h1></div><NewConversationDialog open={creating} onOpenChange={setCreating} orgId={orgId} onCreated={(id) => navigate(`/conversations/${id}`)} /></header>
      <div className="search-field"><Search size={16} /><input aria-label="搜索会话" placeholder="搜索会话" value={filter} onChange={(event) => setFilter(event.target.value)} /></div>
      <div className="filter-tabs"><button className={!filter ? "active" : ""} onClick={() => setFilter("")}>全部</button><button className={filter === "unread" ? "active" : ""} onClick={() => setFilter("unread")}>未读</button><button className={filter === "open" ? "active" : ""} onClick={() => setFilter("open")}>处理中</button></div>
      {conversations.isLoading ? <PageLoading /> : conversations.isError ? <PageError error={conversations.error} /> : <div className="conversation-items">{conversations.data?.map((item) => <Link key={item.id} to={`/conversations/${item.id}`} className={`conversation-item ${selectedId === item.id ? "conversation-item-active" : ""}`}><div className="conversation-avatar">{item.title.slice(0, 1).toUpperCase()}</div><div className="conversation-copy"><div><strong>{item.title}</strong><time>{formatTime(item.last_message_at)}</time></div><p>{item.last_message_preview || item.topic || "暂无消息"}</p><span>{item.priority}</span></div>{item.unread_count > 0 && <b className="unread-count">{item.unread_count}</b>}</Link>)}</div>}
    </aside>
    <main className="message-pane">
      {!selectedId ? <div className="pane-empty"><MessageSquarePlus size={28} /><strong>选择一个会话</strong><span>消息、备注和 Agent 上下文会在这里显示</span></div> : detail.isLoading ? <PageLoading /> : detail.isError ? <PageError error={detail.error} /> : <>
        <header className="workspace-pane-header"><button className="icon-button mobile-only" aria-label="返回会话列表" onClick={() => navigate("/inbox")}><ChevronLeft size={20} /></button><div className="min-w-0"><h2>{detail.data?.conversation.title}</h2><p>{detail.data?.conversation.topic || "无主题"}</p></div><div className="button-row"><button className="button-secondary" disabled={startMeeting.isPending} onClick={() => startMeeting.mutate()}><Video size={16} />开会</button><span className={`status-dot status-${detail.data?.conversation.status}`} /></div></header>
        {pins.data?.length ? <div className="pinned-strip">{pins.data.slice(0, 3).map((message) => <button key={message.id} onClick={() => document.getElementById(`message-${message.id}`)?.scrollIntoView({ block: "center" })}><Pin size={13} /><span>{message.body || "已撤回消息"}</span></button>)}</div> : null}
        <div className="message-stream">
          {messages.hasNextPage && <button className="button-secondary load-older" disabled={messages.isFetchingNextPage} onClick={() => void messages.fetchNextPage()}>加载更早消息</button>}
          {messages.isLoading ? <PageLoading /> : messages.isError ? <PageError error={messages.error} retry={() => void messages.refetch()} /> : messageItems.length ? messageItems.map((message) => <MessageBubble key={message.id} message={message} currentUserId={user?.id} onReply={setReplyTo} onEdit={(item) => { setEditing(item); setComposer(item.body); }} onAction={(action, item, emoji) => messageAction.mutate({ action, message: item, emoji })} />) : <div className="pane-empty"><span>还没有消息</span></div>}
          {activeTypingUsers.length ? <div className="typing-line">对方正在输入...</div> : null}
        </div>
        <form className="message-composer beta-composer" onSubmit={(event) => { event.preventDefault(); if (composer.trim() || attachments.length) send.mutate(); }}>
          {(replyTo || editing || attachments.length > 0) && <div className="composer-context">
            {replyTo && <span><Reply size={13} />回复 {replyTo.sender_display_name || replyTo.sender_email}: {replyTo.body}</span>}
            {editing && <span><Edit3 size={13} />编辑消息 #{editing.id}</span>}
            {attachments.map((item) => <span key={item.id}><Paperclip size={13} />{item.file_name}</span>)}
            <button type="button" className="icon-button" aria-label="清空上下文" onClick={() => { setReplyTo(null); setEditing(null); setAttachments([]); setComposer(""); }}><X size={15} /></button>
          </div>}
          <textarea aria-label="输入消息" placeholder="输入消息" rows={2} value={composer} onChange={(event) => onComposerChange(event.target.value)} />
          <input ref={fileInput} className="hidden" type="file" onChange={(event) => { const file = event.target.files?.[0]; if (file) upload.mutate(file); event.currentTarget.value = ""; }} />
          <button type="button" className="icon-button" aria-label="上传附件" disabled={upload.isPending} onClick={() => fileInput.current?.click()}><Paperclip size={18} /></button>
          <button className="icon-button composer-send" aria-label="发送消息" disabled={(!composer.trim() && attachments.length === 0) || send.isPending}><Send size={18} /></button>
        </form>
      </>}
    </main>
    <aside className="context-pane">
      {!selectedId || !detail.data ? <div className="pane-empty"><Bot size={24} /><span>业务上下文</span></div> : <div className="context-scroll">
        <section className="context-section"><h3>会话状态</h3><label>状态<select className="field" value={detail.data.conversation.status} onChange={(event) => update.mutate({ status: event.target.value })}><option value="open">处理中</option><option value="pending">待处理</option><option value="resolved">已解决</option></select></label><label>优先级<select className="field" value={detail.data.conversation.priority} onChange={(event) => update.mutate({ priority: event.target.value })}><option value="low">低</option><option value="normal">普通</option><option value="high">高</option><option value="urgent">紧急</option></select></label></section>
        <section className="context-section"><h3><Bot size={16} />Agent 上下文</h3><Metric label="会议转写" value={String(detail.data.workspace.agent_context.meeting_transcript_segment_count ?? 0)} /><Metric label="知识来源" value={String(detail.data.workspace.agent_context.knowledge_source_count ?? 0)} /><Metric label="待审批" value={String(detail.data.workspace.agent_context.pending_approval_count ?? 0)} />{detail.data.workspace.agent_context.meeting_transcription_status && <span className="context-status">转写 {detail.data.workspace.agent_context.meeting_transcription_status}</span>}<Link className="button-secondary w-full" to={`/agent-lab?conversationId=${selectedId}`}>打开 Agent Lab</Link>{detail.data.workspace.agent_context.meeting_transcription_status === "ready" && <Link className="button-primary w-full mt-2" to={`/agent-lab?conversationId=${selectedId}&preset=meeting_brief`}>生成会议复盘</Link>}</section>
        <section className="context-section"><h3><Video size={16} />会议</h3><button className="button-secondary w-full" disabled={startMeeting.isPending} onClick={() => startMeeting.mutate()}>从当前会话开会</button></section>
        {detail.data.conversation.latest_recording_id && <section className="context-section"><h3><FileAudio size={16} />最新录音</h3><Link to={`/recordings/${detail.data.conversation.latest_recording_id}`} className="button-secondary w-full">查看转写</Link></section>}
        <section className="context-section"><h3><StickyNote size={16} />内部备注</h3><div className="notes-list">{notes.data?.map((item) => <article key={item.id}><p>{item.body}</p><small>{item.author_display_name} · {formatTime(item.created_at)}</small></article>)}</div><textarea className="field" rows={3} placeholder="仅团队可见" value={note} onChange={(event) => setNote(event.target.value)} /><button className="button-secondary w-full" disabled={!note.trim()} onClick={() => addNote.mutate()}><Check size={16} />添加备注</button></section>
      </div>}
    </aside>
  </div>;
}

function MessageBubble({ message, currentUserId, onReply, onEdit, onAction }: { message: Message; currentUserId?: number; onReply(message: Message): void; onEdit(message: Message): void; onAction(action: "delete" | "pin" | "unpin" | "react", message: Message, emoji?: string): void }) {
  const mine = message.sender_id === currentUserId;
  return <article id={`message-${message.id}`} className={`message-bubble ${message.deleted_at ? "message-deleted" : ""}`}>
    <header><div><strong>{message.sender_display_name || message.sender_email}</strong>{message.edited_at && <span>已编辑</span>}</div><div className="message-actions"><time>{formatTime(message.created_at)}</time><button className="icon-button" title="回复" onClick={() => onReply(message)}><Reply size={14} /></button>{mine && !message.deleted_at && <button className="icon-button" title="编辑" onClick={() => onEdit(message)}><Edit3 size={14} /></button>}<button className="icon-button" title={message.pinned ? "取消置顶" : "置顶"} onClick={() => onAction(message.pinned ? "unpin" : "pin", message)}><Pin size={14} /></button><button className="icon-button" title="赞同" onClick={() => onAction("react", message, "+1")}><MoreHorizontal size={14} /></button>{!message.deleted_at && <button className="icon-button text-danger" title="撤回" onClick={() => onAction("delete", message)}><Trash2 size={14} /></button>}</div></header>
    {message.reply_to && <div className="reply-preview"><Reply size={13} /><span>{message.reply_to.deleted ? "原消息已撤回" : `${message.reply_to.sender_display_name || message.reply_to.sender_email}: ${message.reply_to.body}`}</span></div>}
    <p>{message.deleted_at ? "该消息已撤回" : message.body}</p>
    {message.attachments?.length ? <div className="attachment-list">{message.attachments.map((item) => <a key={item.id} href={item.download_url} target="_blank" rel="noreferrer"><Paperclip size={13} /><span>{item.file_name}</span><small>{formatBytes(item.file_size)}</small></a>)}</div> : null}
    {message.reactions?.length ? <div className="reaction-row">{message.reactions.map((item) => <button key={item.emoji}>{item.emoji} {item.count}</button>)}</div> : null}
  </article>;
}

function Metric({ label, value }: { label: string; value: string }) { return <div className="context-metric"><span>{label}</span><strong>{value}</strong></div>; }

function NewConversationDialog({ open, onOpenChange, orgId, onCreated }: { open: boolean; onOpenChange(open: boolean): void; orgId?: number; onCreated(id: number): void }) {
  const queryClient = useQueryClient(); const [title, setTitle] = useState(""); const [topic, setTopic] = useState("");
  const mutation = useMutation({ mutationFn: () => createConversation({ type: "channel", title, topic }), onSuccess: (item) => { void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "conversations"] }); onOpenChange(false); setTitle(""); setTopic(""); onCreated(item.id); } });
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Trigger asChild><button className="icon-button" aria-label="新建会话"><Plus size={19} /></button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>新建会话</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>创建团队协作上下文，之后可关联联系人、会议和 Agent。</Dialog.Description><FormError error={mutation.error} /><div className="form-stack"><label>标题<input className="field" value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>主题<textarea className="field" value={topic} onChange={(event) => setTopic(event.target.value)} /></label><button className="button-primary" disabled={!title.trim()} onClick={() => mutation.mutate()}>创建会话</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>;
}

function formatBytes(value?: number) {
  if (!value) return "0 B";
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}
