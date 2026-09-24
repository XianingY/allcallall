import { StyleSheet } from "react-native";

export const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f3f4f6",
    paddingTop: 48,
    paddingHorizontal: 20
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 24
  },
  greeting: {
    fontSize: 24,
    fontWeight: "700",
    color: "#111827"
  },
  subtitle: {
    marginTop: 4,
    color: "#6b7280"
  },
  headerButtons: {
    gap: 8
  },
  settingsButton: {
    backgroundColor: "#10b981",
    paddingVertical: 10,
    paddingHorizontal: 12,
    borderRadius: 10
  },
  settingsText: {
    color: "#fff",
    fontWeight: "600",
    fontSize: 12
  },
  changePasswordButton: {
    backgroundColor: "#3b82f6",
    paddingVertical: 10,
    paddingHorizontal: 12,
    borderRadius: 10
  },
  changePasswordText: {
    color: "#fff",
    fontWeight: "600",
    fontSize: 12
  },
  logoutButton: {
    backgroundColor: "#e5e7eb",
    paddingVertical: 10,
    paddingHorizontal: 14,
    borderRadius: 10
  },
  logoutText: {
    color: "#111827",
    fontWeight: "600"
  },
  workspaceText: {
    marginTop: 6,
    color: "#2563eb",
    fontWeight: "600"
  },
  presenceCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 24
  },
  workspaceActionsCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 18
  },
  workspaceActions: {
    flexDirection: "row",
    gap: 10,
    marginTop: 12
  },
  workspaceButton: {
    flex: 1,
    paddingHorizontal: 12,
    paddingVertical: 10
  },
  onboardingCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 18
  },
  followupCard: {
    backgroundColor: "#fff",
    padding: 18,
    borderRadius: 16,
    marginBottom: 18
  },
  followupHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center"
  },
  followupLink: {
    color: "#2563eb",
    fontWeight: "700"
  },
  followupSummary: {
    marginTop: 8,
    color: "#334155",
    fontWeight: "600"
  },
  followupRow: {
    marginTop: 12
  },
  followupPeer: {
    fontWeight: "700",
    color: "#0f172a"
  },
  followupMeta: {
    marginTop: 2,
    color: "#64748b"
  },
  followupEmpty: {
    marginTop: 12,
    color: "#64748b"
  },
  onboardingHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8
  },
  onboardingDescription: {
    color: "#475569",
    marginBottom: 4
  },
  dismissText: {
    color: "#2563eb",
    fontWeight: "700"
  },
  checklistRow: {
    flexDirection: "row",
    alignItems: "center",
    marginTop: 10
  },
  checklistDot: {
    width: 18,
    color: "#94a3b8"
  },
  checklistDotDone: {
    color: "#16a34a"
  },
  checklistText: {
    color: "#334155"
  },
  checklistTextDone: {
    color: "#16a34a",
    fontWeight: "700"
  },
  sectionHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 12
  },
  sectionTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: "#111827"
  },
  addButton: {
    paddingHorizontal: 16,
    paddingVertical: 10
  },
  inviteButton: {
    paddingHorizontal: 16,
    paddingVertical: 10,
    backgroundColor: "#0f172a"
  },
  sectionActions: {
    flexDirection: "row",
    gap: 10
  },
  listContent: {
    paddingBottom: 140
  },
  emptyText: {
    textAlign: "center",
    color: "#6b7280",
    marginTop: 40
  },
  modalBackdrop: {
    flex: 1,
    backgroundColor: "rgba(0,0,0,0.3)",
    justifyContent: "center",
    paddingHorizontal: 20
  },
  modalContent: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 24
  },
  modalTitle: {
    fontSize: 20,
    fontWeight: "700",
    marginBottom: 16
  },
  modalCancel: {
    marginTop: 12,
    backgroundColor: "#9ca3af"
  },
  searchButton: {
    marginBottom: 12
  },
  searchResults: {
    marginBottom: 12
  },
  searchResultRow: {
    borderWidth: 1,
    borderColor: "#e5e7eb",
    borderRadius: 12,
    padding: 12,
    marginBottom: 8
  },
  searchResultTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  searchResultMeta: {
    color: "#64748b",
    marginTop: 4
  },
  reportDescription: {
    color: "#475569",
    marginBottom: 16,
    lineHeight: 20
  },
  reportCategoryList: {
    gap: 10,
    marginBottom: 12
  },
  reportCategoryCard: {
    borderWidth: 1,
    borderColor: "#cbd5e1",
    borderRadius: 14,
    padding: 14,
    backgroundColor: "#f8fafc"
  },
  reportCategoryCardSelected: {
    borderColor: "#2563eb",
    backgroundColor: "#dbeafe"
  },
  reportCategoryTitle: {
    fontWeight: "700",
    color: "#0f172a"
  },
  reportCategoryTitleSelected: {
    color: "#1d4ed8"
  },
  reportCategoryMeta: {
    marginTop: 4,
    color: "#64748b"
  },
  reportCategoryMetaSelected: {
    color: "#1e3a8a"
  },
  reportDetailsInput: {
    minHeight: 96,
    textAlignVertical: "top",
    paddingTop: 12
  },
  actionSheet: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18
  },
  actionSheetButton: {
    paddingVertical: 14
  },
  actionSheetText: {
    fontSize: 16,
    fontWeight: "600",
    color: "#0f172a"
  },
  destructiveText: {
    color: "#dc2626"
  }
});
