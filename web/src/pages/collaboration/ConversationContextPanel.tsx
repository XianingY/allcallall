import { Bot, Check, FileAudio, StickyNote, Video, X } from "lucide-react";
import { memo } from "react";
import { Link } from "react-router-dom";

import { FormError } from "@/components/AuthLayout";
import { SignalTrack } from "@/components/SignalTrack";
import { Metric } from "@/pages/collaboration/InboxParts";
import { formatTime } from "@/pages/collaboration/InboxFormat";
import type { ConversationQueries } from "@/pages/collaboration/useConversationQueries";

interface ConversationContextPanelProps {
  selectedId: number | null;
  detail: ConversationQueries["detail"];
  notes: ConversationQueries["notes"];
  note: string;
  onNoteChange: (value: string) => void;
  onUpdateConversation: (input: { status?: string; priority?: string }) => void;
  onAddNote: () => void;
  onStartMeeting: () => void;
  onClose?: () => void;
  errors?: {
    update?: unknown;
    addNote?: unknown;
    startMeeting?: unknown;
  };
  meetingPending?: boolean;
}

function areContextPanelPropsEqual(
  previous: ConversationContextPanelProps,
  next: ConversationContextPanelProps,
) {
  // TanStack Query returns a fresh result wrapper on every render even when
  // its cached `data` is unchanged. This panel renders only the fields below,
  // so compare those fields instead of the unstable query-result objects.
  return (
    previous.selectedId === next.selectedId &&
    previous.detail.data === next.detail.data &&
    previous.notes.data === next.notes.data &&
    previous.note === next.note &&
    previous.onNoteChange === next.onNoteChange &&
    previous.onUpdateConversation === next.onUpdateConversation &&
    previous.onAddNote === next.onAddNote &&
    previous.onStartMeeting === next.onStartMeeting &&
    previous.onClose === next.onClose &&
    previous.errors?.update === next.errors?.update &&
    previous.errors?.addNote === next.errors?.addNote &&
    previous.errors?.startMeeting === next.errors?.startMeeting &&
    previous.meetingPending === next.meetingPending
  );
}

export const ConversationContextPanel = memo(function ConversationContextPanel({
  selectedId,
  detail,
  notes,
  note,
  onNoteChange,
  onUpdateConversation,
  onAddNote,
  onStartMeeting,
  onClose,
  errors,
  meetingPending,
}: ConversationContextPanelProps) {
  return (
    <aside className="context-pane">
      {!selectedId || !detail.data ? (
        <div className="pane-empty">
          <Bot size={24} />
          <span>业务上下文</span>
        </div>
      ) : (
        <div className="context-scroll">
          <header className="context-header">
            <h2>业务上下文</h2>
            {onClose ? (
              <button className="icon-button" aria-label="关闭上下文" onClick={onClose}>
                <X size={18} />
              </button>
            ) : null}
          </header>
          <section className="context-section">
            <h3>会话状态</h3>
            <label>
              状态
              <select
                className="field"
                value={detail.data.conversation.status}
                onChange={(event) => onUpdateConversation({ status: event.target.value })}
              >
                <option value="open">处理中</option>
                <option value="pending">待处理</option>
                <option value="resolved">已解决</option>
              </select>
            </label>
            <label>
              优先级
              <select
                className="field"
                value={detail.data.conversation.priority}
                onChange={(event) => onUpdateConversation({ priority: event.target.value })}
              >
                <option value="low">低</option>
                <option value="normal">普通</option>
                <option value="high">高</option>
                <option value="urgent">紧急</option>
              </select>
            </label>
            <FormError error={errors?.update} />
          </section>

          <section className="context-section">
            <h3>
              <Bot size={16} />
              Agent 上下文
            </h3>
            {detail.data.workspace.agent_context.meeting_transcription_status ===
            "processing" ? (
              <SignalTrack
                state="connected"
                activity="agent"
                label="Agent 正在整理资料"
              />
            ) : null}
            <Metric
              label="会议转写"
              value={String(
                detail.data.workspace.agent_context.meeting_transcript_segment_count ?? 0,
              )}
            />
            <Metric
              label="知识来源"
              value={String(detail.data.workspace.agent_context.knowledge_source_count ?? 0)}
            />
            <Metric
              label="待审批"
              value={String(detail.data.workspace.agent_context.pending_approval_count ?? 0)}
            />
            {detail.data.workspace.agent_context.meeting_transcription_status ? (
              <span className="context-status">
                转写 {detail.data.workspace.agent_context.meeting_transcription_status}
              </span>
            ) : null}
            <Link
              className="button-secondary w-full"
              to={`/agent-lab?conversationId=${selectedId}`}
            >
              打开 Agent Lab
            </Link>
            {detail.data.workspace.agent_context.meeting_transcription_status === "ready" ? (
              <Link
                className="button-primary w-full mt-2"
                to={`/agent-lab?conversationId=${selectedId}&preset=meeting_brief`}
              >
                生成会议复盘
              </Link>
            ) : null}
          </section>

          <section className="context-section">
            <h3>
              <Video size={16} />
              会议
            </h3>
            <button
              className="button-secondary w-full"
              disabled={meetingPending}
              onClick={onStartMeeting}
            >
              从当前会话开会
            </button>
          </section>

          {detail.data.conversation.latest_recording_id ? (
            <section className="context-section">
              <h3>
                <FileAudio size={16} />
                最新录音
              </h3>
              <Link
                to={`/recordings/${detail.data.conversation.latest_recording_id}`}
                className="button-secondary w-full"
              >
                查看转写
              </Link>
            </section>
          ) : null}

          <section className="context-section">
            <h3>
              <StickyNote size={16} />
              内部备注
            </h3>
            <div className="notes-list">
              {notes.data?.map((item) => (
                <article key={item.id}>
                  <p>{item.body}</p>
                  <small>
                    {item.author_display_name} · {formatTime(item.created_at)}
                  </small>
                </article>
              ))}
            </div>
            <textarea
              className="field"
              rows={3}
              placeholder="仅团队可见"
              value={note}
              onChange={(event) => onNoteChange(event.target.value)}
            />
            <FormError error={errors?.addNote} />
            <button
              className="button-secondary w-full"
              disabled={!note.trim()}
              onClick={onAddNote}
            >
              <Check size={16} />
              添加备注
            </button>
          </section>
        </div>
      )}
    </aside>
  );
}, areContextPanelPropsEqual);
