import { useMutation, useQueryClient } from "@tanstack/react-query";

import {
  addReaction,
  createNote,
  deleteMessage,
  editMessage,
  pinMessage,
  sendMessage,
  type Attachment,
  type Message,
  unpinMessage,
  updateConversation,
  uploadAttachment,
} from "@/api/collaboration";
import { createRoom } from "@/api/meetings";
import { conversationKeys } from "@/pages/collaboration/conversationQueryKeys";

export interface ConversationMutationDraft {
  composer: string;
  note: string;
  replyTo: Message | null;
  editing: Message | null;
  attachments: Attachment[];
}

export function useConversationMutations(input: {
  organizationId: number | undefined;
  conversationId: number | null;
  meetingTitle: string;
  draft: ConversationMutationDraft;
  onMessageSent: () => void;
  onNoteSaved: () => void;
  onAttachmentUploaded: (attachment: Attachment) => void;
  onMeetingStarted: (room: Awaited<ReturnType<typeof createRoom>>) => void;
}) {
  const {
    organizationId,
    conversationId,
    meetingTitle,
    draft,
    onMessageSent,
    onNoteSaved,
    onAttachmentUploaded,
    onMeetingStarted,
  } = input;
  const queryClient = useQueryClient();

  const refreshMessages = () => {
    void queryClient.invalidateQueries({
      queryKey: conversationKeys.messages(organizationId, conversationId),
    });
    void queryClient.invalidateQueries({
      queryKey: conversationKeys.pins(organizationId, conversationId),
    });
    void queryClient.invalidateQueries({
      queryKey: conversationKeys.all(organizationId),
    });
  };

  const send = useMutation({
    mutationFn: () => {
      if (!conversationId) throw new Error("missing conversation");
      if (draft.editing) {
        return editMessage(conversationId, draft.editing.id, draft.composer);
      }
      return sendMessage(conversationId, {
        body: draft.composer,
        reply_to_message_id: draft.replyTo?.id,
        attachment_ids: draft.attachments.map((item) => item.id),
      });
    },
    onSuccess: () => {
      onMessageSent();
      refreshMessages();
    },
  });

  const upload = useMutation({
    mutationFn: (file: File) => uploadAttachment(conversationId!, file),
    onSuccess: onAttachmentUploaded,
  });

  const addNote = useMutation({
    mutationFn: () => createNote(conversationId!, draft.note),
    onSuccess: () => {
      onNoteSaved();
      void queryClient.invalidateQueries({
        queryKey: conversationKeys.notes(organizationId, conversationId),
      });
      void queryClient.invalidateQueries({
        queryKey: conversationKeys.detail(organizationId, conversationId),
      });
    },
  });

  const update = useMutation({
    mutationFn: (change: { status?: string; priority?: string }) =>
      updateConversation(conversationId!, change),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: conversationKeys.detail(organizationId, conversationId),
      });
      void queryClient.invalidateQueries({
        queryKey: conversationKeys.all(organizationId),
      });
    },
  });

  const messageAction = useMutation({
    mutationFn: async (action: {
      action: "delete" | "pin" | "unpin" | "react";
      message: Message;
      emoji?: string;
    }) => {
      if (!conversationId) throw new Error("missing conversation");
      if (action.action === "delete") {
        return deleteMessage(conversationId, action.message.id);
      }
      if (action.action === "pin") return pinMessage(conversationId, action.message.id);
      if (action.action === "unpin") return unpinMessage(conversationId, action.message.id);
      return addReaction(conversationId, action.message.id, action.emoji ?? "+1");
    },
    onSuccess: refreshMessages,
  });

  const startMeeting = useMutation({
    mutationFn: () =>
      createRoom({ title: meetingTitle, conversation_id: conversationId ?? undefined }),
    onSuccess: onMeetingStarted,
  });

  return { send, upload, addNote, update, messageAction, startMeeting };
}

export type ConversationMutations = ReturnType<typeof useConversationMutations>;
