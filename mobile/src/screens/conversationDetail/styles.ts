import { StyleSheet } from "react-native";

export const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16,
  },
  desktopLayout: {
    flex: 1,
    flexDirection: "row",
    gap: 18,
  },
  workspaceColumn: {
    flex: 0.95,
  },
  workspaceColumnContent: {
    paddingBottom: 24,
  },
  messageColumn: {
    flex: 1.1,
  },
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12,
  },
  summaryCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  summaryText: {
    color: "#334155",
    marginTop: 4,
  },
  inlineButton: {
    marginTop: 12,
  },
  inlineButtonSecondary: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  buttonRow: {
    flexDirection: "row",
    gap: 12,
    marginTop: 12,
  },
  button: {
    flex: 1,
  },
  buttonSecondary: {
    flex: 1,
    backgroundColor: "#475569",
  },
  sectionTitle: {
    marginTop: 16,
    marginBottom: 8,
    color: "#0f172a",
    fontWeight: "700",
  },
  optionRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
  },
  option: {
    backgroundColor: "#64748b",
  },
  optionActive: {
    backgroundColor: "#0f172a",
  },
  infoCard: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginTop: 14,
    borderWidth: 1,
    borderColor: "#e2e8f0",
  },
  infoTitle: {
    fontWeight: "700",
    color: "#0f172a",
  },
  agentHeader: {
    flexDirection: "row",
    alignItems: "flex-start",
    justifyContent: "space-between",
    gap: 12,
  },
  agentStatusBadge: {
    backgroundColor: "#0f172a",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 6,
  },
  agentStatusText: {
    color: "#fff",
    fontWeight: "700",
    fontSize: 12,
  },
  agentContextGrid: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 12,
  },
  agentContextItem: {
    color: "#334155",
    backgroundColor: "#f1f5f9",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 6,
    fontSize: 12,
    overflow: "hidden",
  },
  memoryChipRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 12,
  },
  memoryChip: {
    color: "#1e293b",
    backgroundColor: "#e0f2fe",
    borderRadius: 999,
    paddingHorizontal: 10,
    paddingVertical: 6,
    fontSize: 12,
    overflow: "hidden",
  },
  agentResultBox: {
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  infoBody: {
    color: "#334155",
    marginTop: 8,
  },
  infoMeta: {
    color: "#64748b",
    marginTop: 8,
  },
  errorText: {
    color: "#b91c1c",
    marginTop: 8,
  },
  createNoteButton: {
    marginBottom: 12,
  },
  recordingButton: {
    marginTop: 12,
    backgroundColor: "#0f172a",
  },
  systemAction: {
    marginTop: 8,
    paddingVertical: 9,
    backgroundColor: "#0f766e",
  },
  recordingLinkButton: {
    marginTop: 12,
    backgroundColor: "#334155",
  },
  recordingFileRow: {
    marginTop: 12,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  recordingFileTitle: {
    color: "#0f172a",
    fontWeight: "600",
  },
  contactList: {
    gap: 8,
  },
  contactButton: {
    backgroundColor: "#1d4ed8",
  },
  listContent: {
    paddingBottom: 24,
  },
  loadEarlier: {
    alignItems: "center",
    paddingVertical: 12,
  },
  loadEarlierText: {
    color: "#2563eb",
    fontWeight: "600",
  },
  messageBubble: {
    borderRadius: 16,
    padding: 14,
    marginBottom: 12,
    maxWidth: "92%",
  },
  mine: {
    backgroundColor: "#dbeafe",
    alignSelf: "flex-end",
  },
  theirs: {
    backgroundColor: "#fff",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    alignSelf: "flex-start",
  },
  systemBubble: {
    backgroundColor: "#ede9fe",
    alignSelf: "stretch",
  },
  sender: {
    fontWeight: "600",
    color: "#1e293b",
    marginBottom: 6,
  },
  body: {
    color: "#0f172a",
  },
  systemMeta: {
    color: "#6d28d9",
    fontSize: 12,
    marginTop: 8,
  },
  time: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 8,
  },
  tombstoneBubble: {
    alignSelf: "center",
    backgroundColor: "#f1f5f9",
    borderRadius: 12,
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginBottom: 12,
  },
  tombstoneText: {
    color: "#64748b",
    fontSize: 12,
  },
  editedTag: {
    color: "#94a3b8",
    fontSize: 11,
    marginTop: 4,
  },
  attachmentList: {
    marginTop: 8,
    gap: 6,
  },
  attachmentChip: {
    flexDirection: "row",
    alignItems: "center",
    backgroundColor: "#f8fafc",
    borderWidth: 1,
    borderColor: "#e2e8f0",
    borderRadius: 10,
    paddingVertical: 8,
    paddingHorizontal: 10,
    gap: 8,
  },
  attachmentChipBody: {
    flex: 1,
  },
  attachmentName: {
    color: "#0f172a",
    fontSize: 13,
    fontWeight: "600",
  },
  attachmentMeta: {
    color: "#64748b",
    fontSize: 11,
    marginTop: 2,
  },
  attachmentAction: {
    color: "#2563eb",
    fontSize: 12,
    fontWeight: "600",
  },
  pendingAttachmentRow: {
    flexDirection: "row",
    flexWrap: "wrap",
    gap: 8,
    marginTop: 8,
  },
  pendingAttachmentChip: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
    backgroundColor: "#eff6ff",
    borderWidth: 1,
    borderColor: "#bfdbfe",
    borderRadius: 8,
    paddingHorizontal: 10,
    paddingVertical: 6,
  },
  pendingAttachmentName: {
    color: "#1d4ed8",
    fontSize: 12,
    maxWidth: 180,
  },
  pendingAttachmentRemove: {
    color: "#64748b",
    fontSize: 12,
  },
  actionMenuOption: {
    paddingVertical: 14,
    paddingHorizontal: 16,
    borderRadius: 10,
    backgroundColor: "#f8fafc",
  },
  actionMenuOptionText: {
    color: "#0f172a",
    fontSize: 15,
    fontWeight: "600",
  },
  actionMenuOptionDanger: {
    backgroundColor: "#fef2f2",
  },
  actionMenuOptionDangerText: {
    color: "#b91c1c",
  },
  noteRow: {
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
    paddingTop: 10,
    marginTop: 10,
  },
  noteBody: {
    color: "#334155",
  },
  citationList: {
    marginTop: 12,
    gap: 10,
  },
  citationItem: {
    paddingTop: 10,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  citationHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    gap: 8,
  },
  citationBadge: {
    color: "#075985",
    backgroundColor: "#e0f2fe",
    borderRadius: 999,
    paddingHorizontal: 8,
    paddingVertical: 3,
    fontSize: 11,
    fontWeight: "700",
    overflow: "hidden",
  },
  approvalItem: {
    paddingTop: 10,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  inlineActionRow: {
    flexDirection: "row",
    gap: 10,
    marginTop: 10,
  },
  approveChip: {
    backgroundColor: "#166534",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  approveChipText: {
    color: "#fff",
    fontWeight: "600",
  },
  rejectChip: {
    backgroundColor: "#991b1b",
    borderRadius: 999,
    paddingHorizontal: 12,
    paddingVertical: 8,
  },
  rejectChipText: {
    color: "#fff",
    fontWeight: "600",
  },
  citationTitle: {
    color: "#0f172a",
    fontWeight: "600",
  },
  citationMeta: {
    color: "#475569",
    marginTop: 4,
    fontSize: 12,
  },
  citationSnippet: {
    color: "#334155",
    marginTop: 6,
  },
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(15, 23, 42, 0.45)",
    justifyContent: "center",
    padding: 18,
  },
  modalCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    maxHeight: "80%",
  },
  debugDrawer: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    maxHeight: "88%",
  },
  modalTitle: {
    color: "#0f172a",
    fontWeight: "700",
    fontSize: 18,
  },
  modalScroll: {
    marginTop: 12,
  },
  modalSection: {
    paddingTop: 12,
    marginTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#e2e8f0",
  },
  modalButton: {
    marginTop: 16,
  },
  linkRow: {
    marginTop: 12,
  },
  linkText: {
    color: "#2563eb",
    fontWeight: "600",
  },
  debugHeader: {
    marginTop: 16,
    color: "#0f172a",
    fontWeight: "700",
  },
  composer: {
    marginTop: 12,
  },
});

export default styles;
