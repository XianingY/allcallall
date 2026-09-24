import React from "react";
import { Platform, Pressable, Text, View } from "react-native";

import {
  type AgentCitation,
  type AgentRunEventRecord,
  type AgentToolCallRecord,
  type AgentTraceEventRecord,
} from "../../api/agent";
import { compact, formatTime, jsonSummary, sourceTypeLabel } from "../agentDemoUtils";
import { styles } from "./styles";

const statusTone = (status: string) => {
  switch (status) {
    case "ready":
    case "executed":
    case "indexed":
    case "active":
      return styles.statusReady;
    case "running":
    case "pending":
    case "requires_action":
      return styles.statusRunning;
    case "failed":
    case "rejected":
      return styles.statusFailed;
    default:
      return styles.statusNeutral;
  }
};

interface StatusPillProps {
  status: string;
}

interface SignalListProps {
  title: string;
  items?: string[];
}

export const SignalList: React.FC<SignalListProps> = ({ title, items }) => {
  if (!items?.length) return null;
  return (
    <View style={styles.signalBlock}>
      <Text style={styles.signalTitle}>{title}</Text>
      <View style={styles.chipRow}>
        {items.map((item, index) => (
          <Text key={`${title}-${index}-${item}`} style={styles.signalChip}>
            {item}
          </Text>
        ))}
      </View>
    </View>
  );
};

interface TraceTimelineProps {
  trace: AgentTraceEventRecord[];
  events: AgentRunEventRecord[];
}

export const TraceTimeline: React.FC<TraceTimelineProps> = ({ trace, events }) => {
  if (trace.length === 0 && events.length === 0) return null;
  return (
    <View style={styles.traceBlock}>
      <Text style={styles.citationsTitle}>Trace</Text>
      {trace.map((item, index) => (
        <View
          key={`trace-${index}-${item.ref_id ?? item.name}`}
          style={styles.timelineItem}
        >
          <View style={styles.timelineDot} />
          <View style={styles.timelineBody}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{item.name || item.type}</Text>
              <StatusPill status={item.status} />
            </View>
            <Text style={styles.rowMeta}>
              {item.type} · {formatTime(item.at)}
            </Text>
            {item.metadata ? (
              <Text style={styles.messageBody}>
                {compact(JSON.stringify(item.metadata), 260)}
              </Text>
            ) : null}
          </View>
        </View>
      ))}
      {events.map((event) => (
        <View key={`event-${event.sequence}`} style={styles.timelineItem}>
          <View style={styles.timelineDotMuted} />
          <View style={styles.timelineBody}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{event.name || event.event}</Text>
              <StatusPill status={event.status} />
            </View>
            <Text style={styles.rowMeta}>
              event #{event.sequence} · {event.ref_type || "run"} ·{" "}
              {formatTime(event.at)}
            </Text>
            {event.metadata ? (
              <Text style={styles.messageBody}>
                {compact(JSON.stringify(event.metadata), 260)}
              </Text>
            ) : null}
          </View>
        </View>
      ))}
    </View>
  );
};

interface ToolCallListProps {
  toolCalls: AgentToolCallRecord[];
}

export const ToolCallList: React.FC<ToolCallListProps> = ({ toolCalls }) => {
  if (toolCalls.length === 0) return null;
  return (
    <View style={styles.traceBlock}>
      <Text style={styles.citationsTitle}>Tool Calls</Text>
      {toolCalls.map((call) => (
        <View key={call.id} style={styles.toolCallItem}>
          <View style={styles.rowTop}>
            <Text style={styles.rowTitle}>{call.tool_name}</Text>
            <StatusPill status={call.status} />
          </View>
          <Text style={styles.rowMeta}>
            call #{call.id}
            {call.step_id ? ` · step #${call.step_id}` : ""}
          </Text>
          {call.input_json ? (
            <Text style={styles.messageBody}>
              Input: {jsonSummary(call.input_json, 300)}
            </Text>
          ) : null}
          {call.output_json ? (
            <Text style={styles.messageBody}>
              Output: {jsonSummary(call.output_json, 360)}
            </Text>
          ) : null}
          {call.error_message ? (
            <Text style={styles.errorText}>
              {compact(call.error_message, 260)}
            </Text>
          ) : null}
        </View>
      ))}
    </View>
  );
};

export const StatusPill: React.FC<StatusPillProps> = ({ status }) => (
  <View style={[styles.statusPill, statusTone(status)]}>
    <Text style={styles.statusText}>{status || "unknown"}</Text>
  </View>
);

interface CitationListProps {
  citations: AgentCitation[];
  onPress: (citation: AgentCitation) => void;
}

export const CitationList: React.FC<CitationListProps> = ({ citations, onPress }) => {
  if (citations.length === 0) return null;
  const grouped = citations
    .slice(0, 12)
    .reduce<Array<{ sourceType: string; items: AgentCitation[] }>>(
      (groups, citation) => {
        const existing = groups.find(
          (group) => group.sourceType === citation.source_type,
        );
        if (existing) {
          existing.items.push(citation);
        } else {
          groups.push({ sourceType: citation.source_type, items: [citation] });
        }
        return groups;
      },
      [],
    );
  return (
    <View style={styles.citationContainer}>
      <Text style={styles.citationsTitle}>Citations</Text>
      {grouped.map((group) => (
        <View key={group.sourceType} style={styles.citationGroup}>
          <Text style={styles.sourceBadge}>
            {sourceTypeLabel(group.sourceType)}
          </Text>
          {group.items.map((citation) => (
            <Pressable
              key={`${citation.source_type}:${citation.source_id}:${citation.chunk_id ?? ""}`}
              style={styles.citationItem}
              onPress={() => onPress(citation)}
            >
              <View style={styles.rowTop}>
                <Text style={styles.citationTitle}>
                  {citation.source_title || citation.title}
                </Text>
                <Text style={styles.scoreText}>
                  {citation.retrieval_mode || "source"} · {citation.score}
                  {citation.rrf_score
                    ? ` · rrf ${citation.rrf_score.toFixed(3)}`
                    : ""}
                </Text>
              </View>
              <Text style={styles.citationSnippet}>{citation.snippet}</Text>
            </Pressable>
          ))}
        </View>
      ))}
    </View>
  );
};

interface WebFilePickerProps {
  onFile: (file: File) => void;
}

export const WebFilePicker: React.FC<WebFilePickerProps> = ({ onFile }) => {
  if (Platform.OS !== "web") return null;
  return React.createElement("input", {
    type: "file",
    accept:
      ".txt,.md,.html,.pdf,text/plain,text/markdown,text/html,application/pdf",
    style: {
      border: "1px solid #cbd5e1",
      borderRadius: 8,
      padding: 12,
      width: "100%",
      boxSizing: "border-box",
      marginTop: 8,
      color: "#0f172a",
    },
    onChange: (event: React.ChangeEvent<HTMLInputElement>) => {
      const file = event.target.files?.[0];
      if (file) onFile(file);
      event.target.value = "";
    },
  });
};
