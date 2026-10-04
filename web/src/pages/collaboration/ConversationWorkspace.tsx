import {
  Bot,
  ChevronLeft,
  Edit3,
  MessageSquarePlus,
  Paperclip,
  Pin,
  Reply,
  Send,
  Video,
  X,
} from "lucide-react";
import { useRef } from "react";
import type { RefObject } from "react";

import type { Attachment, Message } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { MessageBubble } from "@/pages/collaboration/InboxParts";
import type { ConversationQueries } from "@/pages/collaboration/useConversationQueries";
import type { ConversationMutations } from "@/pages/collaboration/useConversationMutations";

interface ConversationWorkspaceProps {
  selectedId: number | null;
  currentUserId?: number;
  queries: ConversationQueries;
  mutations: ConversationMutations;
  draft: {
    composer: string;
    replyTo: Message | null;
    editing: Message | null;
    attachments: Attachment[];
  };
  typingUsers: number[];
  onComposerChange: (value: string) => void;
  onSubmit: () => void;
  onSetReplyTo: (message: Message) => void;
  onSetEditing: (message: Message) => void;
  onClearComposerContext: () => void;
  onBackToList: () => void;
  contextOpen: boolean;
  onToggleContext: () => void;
  contextButtonRef?: RefObject<HTMLButtonElement>;
}

export function ConversationWorkspace({
  selectedId,
  currentUserId,
  queries,
  mutations,
  draft,
  typingUsers,
  onComposerChange,
  onSubmit,
  onSetReplyTo,
  onSetEditing,
  onClearComposerContext,
  onBackToList,
  contextOpen,
  onToggleContext,
  contextButtonRef,
}: ConversationWorkspaceProps) {
  const fileInput = useRef<HTMLInputElement>(null);
  const { detail, messages, pins, messageItems, messageWindow } = queries;
  const { send, upload, messageAction, startMeeting } = mutations;

  return (
    <main className="message-pane">
      {!selectedId ? (
        <div className="pane-empty">
          <MessageSquarePlus size={28} />
          <strong>选择一个会话</strong>
          <span>消息、备注和 Agent 上下文会在这里显示</span>
        </div>
      ) : detail.isLoading ? (
        <PageLoading />
      ) : detail.isError ? (
        <PageError error={detail.error} />
      ) : (
        <>
          <header className="workspace-pane-header">
            <button
              className="icon-button mobile-only"
              aria-label="返回会话列表"
              onClick={onBackToList}
            >
              <ChevronLeft size={20} />
            </button>
            <div className="min-w-0">
              <h2>{detail.data?.conversation.title}</h2>
              <p>{detail.data?.conversation.topic || "无主题"}</p>
            </div>
            <div className="button-row">
              <button
                ref={contextButtonRef}
                className="icon-button context-toggle"
                aria-label="业务上下文"
                aria-expanded={contextOpen}
                aria-controls="inbox-context-region"
                onClick={onToggleContext}
              >
                <Bot size={18} />
              </button>
              <button
                className="button-secondary"
                disabled={startMeeting.isPending}
                onClick={() => startMeeting.mutate()}
              >
                <Video size={16} />
                开会
              </button>
              <span className={`status-dot status-${detail.data?.conversation.status}`} />
            </div>
          </header>

          <FormError error={startMeeting.error} />

          {pins.data?.length ? (
            <div className="pinned-strip">
              {pins.data.slice(0, 3).map((message) => (
                <button
                  key={message.id}
                  onClick={() =>
                    document
                      .getElementById(`message-${message.id}`)
                      ?.scrollIntoView({ block: "center" })
                  }
                >
                  <Pin size={13} />
                  <span>{message.body || "已撤回消息"}</span>
                </button>
              ))}
            </div>
          ) : null}

          <div className="message-stream">
            {messages.hasNextPage ? (
              <button
                className="button-secondary load-older"
                disabled={messages.isFetchingNextPage}
                onClick={() => void messages.fetchNextPage()}
              >
                加载更早消息
              </button>
            ) : null}
            {messageWindow.hiddenCount > 0 ? (
              <div className="windowed-message-note">
                已折叠 {messageWindow.hiddenCount} 条较早消息，使用搜索或继续加载定位历史内容。
              </div>
            ) : null}
            {messages.isLoading ? (
              <PageLoading />
            ) : messages.isError ? (
              <PageError error={messages.error} retry={() => void messages.refetch()} />
            ) : messageItems.length ? (
              messageWindow.visible.map((message) => (
                <MessageBubble
                  key={message.id}
                  message={message}
                  currentUserId={currentUserId}
                  onReply={onSetReplyTo}
                  onEdit={onSetEditing}
                  onAction={(action, item, emoji) =>
                    messageAction.mutate({ action, message: item, emoji })
                  }
                />
              ))
            ) : (
              <div className="pane-empty">
                <span>还没有消息</span>
              </div>
            )}
            {typingUsers.length ? (
              <div className="typing-line" role="status" aria-live="polite">
                对方正在输入...
              </div>
            ) : null}
          </div>

          <FormError error={messageAction.error} />

          <form
            className="message-composer beta-composer"
            onSubmit={(event) => {
              event.preventDefault();
              if (draft.composer.trim() || draft.attachments.length) onSubmit();
            }}
          >
            {draft.replyTo || draft.editing || draft.attachments.length > 0 ? (
              <div className="composer-context">
                {draft.replyTo ? (
                  <span>
                    <Reply size={13} />
                    回复 {draft.replyTo.sender_display_name || draft.replyTo.sender_email}:{" "}
                    {draft.replyTo.body}
                  </span>
                ) : null}
                {draft.editing ? (
                  <span>
                    <Edit3 size={13} />
                    编辑消息 #{draft.editing.id}
                  </span>
                ) : null}
                {draft.attachments.map((item) => (
                  <span key={item.id}>
                    <Paperclip size={13} />
                    {item.file_name}
                  </span>
                ))}
                <button
                  type="button"
                  className="icon-button"
                  aria-label="清空上下文"
                  onClick={onClearComposerContext}
                >
                  <X size={15} />
                </button>
              </div>
            ) : null}

            <textarea
              aria-label="输入消息"
              placeholder="输入消息"
              rows={2}
              value={draft.composer}
              onChange={(event) => onComposerChange(event.target.value)}
            />
            <input
              ref={fileInput}
              className="hidden"
              type="file"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) upload.mutate(file);
                event.currentTarget.value = "";
              }}
            />
            <FormError error={upload.error} />
            <FormError error={send.error} />
            <button
              type="button"
              className="icon-button"
              aria-label="上传附件"
              disabled={upload.isPending}
              onClick={() => fileInput.current?.click()}
            >
              <Paperclip size={18} />
            </button>
            <button
              type="button"
              className="icon-button composer-send"
              aria-label="发送消息"
              disabled={
                (!draft.composer.trim() && draft.attachments.length === 0) || send.isPending
              }
              onClick={onSubmit}
            >
              <Send size={18} />
            </button>
          </form>
        </>
      )}
    </main>
  );
}
