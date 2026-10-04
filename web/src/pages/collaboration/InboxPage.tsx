import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";

import {
  markConversationRead,
  type Attachment,
  type Message,
} from "@/api/collaboration";
import { useAuth } from "@/auth/AuthContext";
import { useOrganization } from "@/organizations/OrganizationContext";
import { ConversationContextPanel } from "@/pages/collaboration/ConversationContextPanel";
import { ConversationSidebar } from "@/pages/collaboration/ConversationSidebar";
import { ConversationWorkspace } from "@/pages/collaboration/ConversationWorkspace";
import { conversationKeys } from "@/pages/collaboration/conversationQueryKeys";
import {
  useConversationQueries,
  type ConversationFilterState,
} from "@/pages/collaboration/useConversationQueries";
import { useConversationMutations } from "@/pages/collaboration/useConversationMutations";
import { useTypingSignal } from "@/pages/collaboration/useTypingSignal";

export function InboxPage() {
  const { conversationId } = useParams();
  const selectedId = Number(conversationId) || null;
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { user } = useAuth();
  const { activeOrganization } = useOrganization();
  const orgId = activeOrganization?.id;

  // `status` is a server-side filter; `keyword` and `unreadOnly` filter loaded
  // pages locally. Message-body search is committed only on Enter.
  const [filter, setFilter] = useState<ConversationFilterState>({
    status: "",
    keyword: "",
    unreadOnly: false,
    messageQuery: "",
  });
  const [composer, setComposer] = useState("");
  const [note, setNote] = useState("");
  const [replyTo, setReplyTo] = useState<Message | null>(null);
  const [editing, setEditing] = useState<Message | null>(null);
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const [typingUsers, setTypingUsers] = useState<Record<number, number>>({});
  const [typingClock, setTypingClock] = useState(0);
  const [isNarrow, setIsNarrow] = useState(false);
  const [contextOpen, setContextOpen] = useState(true);
  const [mobileContextConversationId, setMobileContextConversationId] = useState<number | null>(
    null,
  );
  const contextButtonRef = useRef<HTMLButtonElement>(null);

  const { signalTyping, stopTyping } = useTypingSignal({
    conversationId: selectedId,
    onBeforeStop: () => setComposer(""),
  });

  const queries = useConversationQueries({
    organizationId: orgId,
    conversationId: selectedId,
    filter,
  });

  const mutations = useConversationMutations({
    organizationId: orgId,
    conversationId: selectedId,
    meetingTitle: queries.detail.data?.conversation.title || "Team Meeting",
    draft: { composer, note, replyTo, editing, attachments },
    onMessageSent: () => {
      stopTyping();
      setComposer("");
      setReplyTo(null);
      setEditing(null);
      setAttachments([]);
    },
    onNoteSaved: () => setNote(""),
    onAttachmentUploaded: (item) => setAttachments((items) => [...items, item]),
    onMeetingStarted: (room) => navigate(`/meetings/${room.room.id}/preflight`),
  });

  const activeTypingUsers = useMemo(
    () =>
      Object.entries(typingUsers)
        .filter(([id, until]) => Number(id) !== user?.id && (typingClock === 0 || until > typingClock))
        .map(([id]) => Number(id)),
    [typingClock, typingUsers, user?.id],
  );

  useEffect(() => {
    const query = window.matchMedia("(max-width: 900px)");
    const updateNarrow = () => setIsNarrow(query.matches);
    updateNarrow();
    query.addEventListener("change", updateNarrow);
    return () => query.removeEventListener("change", updateNarrow);
  }, []);

  useEffect(() => {
    if (!Object.values(typingUsers).some((until) => until > 0)) return;
    const timer = window.setInterval(() => setTypingClock(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [typingUsers]);

  useEffect(() => {
    queueMicrotask(() => {
      setReplyTo(null);
      setEditing(null);
      setAttachments([]);
    });
    if (selectedId) {
      void markConversationRead(selectedId).then(() =>
        queryClient.invalidateQueries({ queryKey: conversationKeys.all(orgId) }),
      );
    }
  }, [selectedId, orgId, queryClient]);

  useEffect(() => {
    const listener = (event: Event) => {
      const detail = event as CustomEvent<{
        event: string;
        payload: { conversation_id?: number; user_id?: number; typing?: boolean };
      }>;
      const payload = detail.detail.payload;
      if (!selectedId || payload.conversation_id !== selectedId || !payload.user_id) return;
      if (!detail.detail.event.startsWith("typing.")) return;
      setTypingUsers((current) => ({
        ...current,
        [payload.user_id!]: payload.typing ? Date.now() + 3000 : 0,
      }));
    };
    window.addEventListener("allcallall:chat-event", listener);
    return () => window.removeEventListener("allcallall:chat-event", listener);
  }, [selectedId]);

  const onFilterChange = useCallback((next: Partial<ConversationFilterState>) => {
    setFilter((current) => ({ ...current, ...next }));
  }, []);

  const onOpenConversation = useCallback((conversationId: number) => {
    setMobileContextConversationId(null);
    navigate(`/conversations/${conversationId}`);
  }, [navigate]);

  const onSetEditing = useCallback((message: Message) => {
    setEditing(message);
    setComposer(message.body);
  }, []);

  const onBackToList = useCallback(() => {
    setMobileContextConversationId(null);
    navigate("/inbox");
  }, [navigate]);

  const onComposerChange = (value: string) => {
    setComposer(value);
    if (value.trim()) signalTyping(value);
    else stopTyping();
  };

  const onSubmit = () => {
    if (composer.trim() || attachments.length) mutations.send.mutate();
  };

  const clearComposerContext = () => {
    stopTyping();
    setReplyTo(null);
    setEditing(null);
    setAttachments([]);
    setComposer("");
  };

  const mobileRegion =
    mobileContextConversationId !== null && mobileContextConversationId === selectedId
      ? "context"
      : selectedId
        ? "conversation"
        : "list";

  const closeContext = useCallback(() => {
    setContextOpen(false);
    setMobileContextConversationId(null);
    window.requestAnimationFrame(() => contextButtonRef.current?.focus());
  }, []);

  const toggleContext = () => {
    if (isNarrow) {
      setContextOpen(true);
      setMobileContextConversationId((current) => (current === selectedId ? null : selectedId));
      return;
    }
    setContextOpen((open) => !open);
  };

  const regionProps = (region: "list" | "conversation" | "context") => ({
    className: `inbox-region inbox-region-${region}`,
    hidden:
      (isNarrow && mobileRegion !== region) ||
      (!isNarrow && region === "context" && !contextOpen),
    inert:
      (isNarrow && mobileRegion !== region) ||
      (!isNarrow && region === "context" && !contextOpen)
        ? ""
        : undefined,
  });

  return (
    <div
      className={`inbox-layout ${selectedId ? "inbox-selected" : ""}`}
      data-mobile-region={mobileRegion}
    >
      <div {...regionProps("list")}>
        <ConversationSidebar
          filter={filter}
          onFilterChange={onFilterChange}
          queries={queries}
          selectedId={selectedId}
          organizationId={orgId}
          onCreateConversation={noop}
          onOpenConversation={onOpenConversation}
        />
      </div>

      <div {...regionProps("conversation")}>
        <ConversationWorkspace
          selectedId={selectedId}
          currentUserId={user?.id}
          queries={queries}
          mutations={mutations}
          draft={{ composer, replyTo, editing, attachments }}
          typingUsers={activeTypingUsers}
          onComposerChange={onComposerChange}
          onSubmit={onSubmit}
          onSetReplyTo={setReplyTo}
          onSetEditing={onSetEditing}
          onClearComposerContext={clearComposerContext}
          onBackToList={onBackToList}
          contextOpen={contextOpen}
          onToggleContext={toggleContext}
          contextButtonRef={contextButtonRef}
        />
      </div>

      <div id="inbox-context-region" {...regionProps("context")}>
        <ConversationContextPanel
          selectedId={selectedId}
          detail={queries.detail}
          notes={queries.notes}
          note={note}
          onNoteChange={setNote}
          onUpdateConversation={mutations.update.mutate}
          onAddNote={mutations.addNote.mutate}
          onStartMeeting={mutations.startMeeting.mutate}
          onClose={isNarrow ? closeContext : undefined}
          errors={{
            update: mutations.update.error,
            addNote: mutations.addNote.error,
            startMeeting: mutations.startMeeting.error,
          }}
          meetingPending={mutations.startMeeting.isPending}
        />
      </div>
    </div>
  );
}

const noop = () => undefined;
