import React from "react";
import { Text, View } from "react-native";

import type {
  MCPExecution,
  MCPInstallation,
  MCPTool
} from "../../api/mcpPlatform";
import { formatPlatformJSON, toolExecutionPolicy } from "../mcpPlatformUtils";

import { ActionButton, Field, StatusBadge } from "./primitives";
import { styles } from "./styles";

interface ExecutionPanelProps {
  executionID: string;
  setExecutionID: React.Dispatch<React.SetStateAction<string>>;
  busyKey: string;
  onFetchExecution: () => void;
  execution: MCPExecution | null;
  executionTool: MCPTool | null;
  executionInstallation: MCPInstallation | null;
}

export const ExecutionPanel: React.FC<ExecutionPanelProps> = ({
  executionID,
  setExecutionID,
  busyKey,
  onFetchExecution,
  execution,
  executionTool,
  executionInstallation
}) => (
          <>
            <View style={styles.sectionHeader}>
              <View>
                <Text style={styles.sectionTitle}>执行结果</Text>
                <Text style={styles.sectionMeta}>按 execution ID 查询</Text>
              </View>
            </View>
            <View style={styles.executionSearch}>
              <View style={styles.flexCopy}>
                <Field
                  label="Execution ID"
                  value={executionID}
                  autoCapitalize="none"
                  onChangeText={setExecutionID}
                  placeholder="mcp:..."
                />
              </View>
              <ActionButton
                label={busyKey === "execution" ? "查询中..." : "查询"}
                compact
                disabled={busyKey !== "" || !executionID.trim()}
                onPress={() => void onFetchExecution()}
              />
            </View>
            {execution ? (
              <View style={styles.executionDetail}>
                <View style={styles.rowBetween}>
                  <View style={styles.flexCopy}>
                    <Text style={styles.detailTitle}>{execution.execution_id}</Text>
                    <Text style={styles.itemMeta}>
                      {executionTool?.name ?? `Tool #${execution.tool_id}`} · revision #{execution.revision_id} · attempts {execution.attempts}
                    </Text>
                  </View>
                  <StatusBadge status={execution.status} />
                </View>
                {execution.error_message ? (
                  <Text style={styles.warningText}>{execution.error_message}</Text>
                ) : null}
                <View style={styles.detailGrid}>
                  <Text style={styles.detailKey}>来源</Text>
                  <Text style={styles.detailValue}>
                    {executionInstallation?.source_type.toUpperCase() ??
                      `Installation #${execution.installation_id}`}
                  </Text>
                  <Text style={styles.detailKey}>风险</Text>
                  <Text style={styles.detailValue}>
                    {executionTool?.risk ?? "unknown"}
                  </Text>
                </View>
                <Text style={styles.approvalPolicy}>
                  {executionTool
                    ? toolExecutionPolicy(executionTool.risk)
                    : "工具风险和审批策略由 Go 网关判定"}
                </Text>
                <Text style={styles.payloadHeading}>输入</Text>
                <Text style={styles.payload}>{formatPlatformJSON(execution.input)}</Text>
                <View style={styles.untrustedHeader}>
                  <Text style={styles.payloadHeading}>工具输出</Text>
                  <Text style={styles.untrustedLabel}>不可信数据</Text>
                </View>
                <Text style={styles.payload}>{formatPlatformJSON(execution.output)}</Text>
              </View>
            ) : null}
          </>
);
