import { Pressable, ScrollView, Text, View } from "react-native";
import { StatusPill } from "./components";
import { compact, jsonSummary, type ApprovalFilter } from "../agentDemoUtils";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const ApprovalsTab = ({ controller }: { controller: AgentLabController }) => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <View style={styles.panelHeader}>
          <View>
            <Text style={styles.sectionTitle}>Tool Approvals</Text>
            <Text style={styles.contextLine}>
              {controller.approvals.filter((item) => item.status === "pending").length}{" "}
              pending · {controller.approvals.length} total
            </Text>
          </View>
          <View style={styles.segmentRowCompact}>
            {(["pending", "all"] as ApprovalFilter[]).map((filter) => {
              const selected = controller.approvalFilter === filter;
              return (
                <Pressable
                  key={filter}
                  style={[
                    styles.segmentButton,
                    selected && styles.segmentActive,
                  ]}
                  onPress={() => controller.setApprovalFilter(filter)}
                >
                  <Text
                    style={[
                      styles.segmentText,
                      selected && styles.segmentTextActive,
                    ]}
                  >
                    {filter}
                  </Text>
                </Pressable>
              );
            })}
          </View>
        </View>
        {controller.visibleApprovals.map((approval) => (
          <View key={approval.id} style={styles.approvalBox}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{approval.tool_name}</Text>
              <StatusPill status={approval.status} />
            </View>
            <Text style={styles.rowMeta}>
              workflow #{approval.workflow_run_id} · requested by{" "}
              {approval.requested_by}
              {approval.decided_by
                ? ` · decided by ${approval.decided_by}`
                : ""}
            </Text>
            {approval.mcp_revision_id && approval.mcp_revision_id > 0 ? (
              <Text style={styles.rowMeta}>
                {approval.mcp_installation_id
                  ? `MCP Installation #${approval.mcp_installation_id}`
                  : "MCP"}{" "}
                · Revision #{approval.mcp_revision_id}
              </Text>
            ) : null}
            <Text style={styles.rowMeta}>
              Schema {approval.tool_schema_version || "-"}
              {approval.approval_request_id
                ? ` · Approval ${approval.approval_request_id} · checkpoint v${approval.approval_checkpoint_version}`
                : ""}
            </Text>
            <Text style={styles.messageBody}>
              Input: {jsonSummary(approval.input_json, 360)}
            </Text>
            {approval.output_json ? (
              <Text style={styles.messageBody}>
                Output: {jsonSummary(approval.output_json, 280)}
              </Text>
            ) : null}
            {approval.decision ? (
              <Text style={styles.rowMeta}>Decision: {approval.decision}</Text>
            ) : null}
            {approval.error_message ? (
              <Text style={styles.errorText}>
                {compact(approval.error_message, 280)}
              </Text>
            ) : null}
            {approval.status === "pending" ? (
              <View style={styles.inlineActions}>
                <Pressable
                  style={styles.approveButton}
                  onPress={() => void controller.handleApproval(approval, "approve")}
                >
                  <Text style={styles.approveButtonText}>Approve</Text>
                </Pressable>
                <Pressable
                  style={styles.rejectButton}
                  onPress={() => void controller.handleApproval(approval, "reject")}
                >
                  <Text style={styles.rejectButtonText}>Reject</Text>
                </Pressable>
              </View>
            ) : null}
          </View>
        ))}
        {controller.visibleApprovals.length === 0 ? (
          <Text style={styles.emptyText}>No approvals.</Text>
        ) : null}
      </View>
    </ScrollView>
  );
