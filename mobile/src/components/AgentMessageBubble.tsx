import React, { useCallback, useEffect, useMemo, useState } from "react";
import { ActivityIndicator, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import type EventSource from "react-native-sse";

import {
  createAgentRunEventSource,
  fetchAgentRun,
  type AgentCitation,
  type AgentRunEventRecord,
  type AgentRunResult,
} from "../api/agent";
import { useAuthContext } from "../context/AuthContext";

interface Props {
  runId: number;
  initialResult?: AgentRunResult | null;
  onComplete?: () => void;
}

const terminalEvents = new Set(["run_ready", "run_failed"]);
const terminalStatuses = new Set(["ready", "failed"]);

const eventLabel = (event: AgentRunEventRecord) => {
  if (event.name) return event.name;
  return event.event.replace(/_/g, " ");
};

const statusLabel = (status: string) => {
  switch (status) {
    case "pending":
      return "Queued";
    case "running":
      return "Running";
    case "ready":
      return "Ready";
    case "failed":
      return "Failed";
    case "requires_action":
      return "Needs approval";
    default:
      return status || "Unknown";
  }
};

const parseEventPayload = (eventName: string, raw: string): AgentRunEventRecord | null => {
  try {
    const parsed = JSON.parse(raw) as Partial<AgentRunEventRecord>;
    if (!parsed || typeof parsed !== "object") return null;
    return {
      sequence: Number(parsed.sequence ?? 0),
      event: String(parsed.event ?? eventName),
      status: String(parsed.status ?? ""),
      ref_type: String(parsed.ref_type ?? ""),
      ref_id: parsed.ref_id ? Number(parsed.ref_id) : undefined,
      name: parsed.name ? String(parsed.name) : undefined,
      at: String(parsed.at ?? new Date().toISOString()),
      metadata: parsed.metadata,
    };
  } catch {
    return null;
  }
};

export const AgentMessageBubble: React.FC<Props> = ({ runId, initialResult, onComplete }) => {
  const { token } = useAuthContext();
  const [result, setResult] = useState<AgentRunResult | null>(initialResult ?? null);
  const [events, setEvents] = useState<AgentRunEventRecord[]>([]);
  const [tokens, setTokens] = useState("");
  const [status, setStatus] = useState(initialResult?.run.status ?? "pending");
  const [expanded, setExpanded] = useState(true);

  const refreshRun = useCallback(async () => {
    if (!token) return null;
    const next = await fetchAgentRun(token, runId);
    setResult(next);
    setStatus(next.run.status);
    return next;
  }, [runId, token]);

  useEffect(() => {
    setResult(initialResult ?? null);
    setStatus(initialResult?.run.status ?? "pending");
  }, [initialResult, runId]);

  useEffect(() => {
    if (!token) return;
    void refreshRun().catch((error) => {
      console.error("[AgentMessageBubble] Failed to fetch run:", error);
      setStatus("failed");
    });
  }, [refreshRun, token]);

  useEffect(() => {
    if (!token || terminalStatuses.has(initialResult?.run.status ?? "")) return;

    let es: EventSource | null = null;
    let mounted = true;

    try {
      es = createAgentRunEventSource(token, runId);

      const bindRunEvent = (eventName: string) => {
        es?.addEventListener(eventName as never, (event: MessageEvent) => {
          if (!mounted || !event.data) return;
          const payload = parseEventPayload(eventName, String(event.data));
          if (!payload) return;
          setEvents((previous) => {
            const bySequence = payload.sequence > 0
              ? previous.filter((item) => item.sequence !== payload.sequence)
              : previous;
            return [...bySequence, payload].sort((a, b) => a.sequence - b.sequence);
          });
          if (payload.status) {
            setStatus(payload.status);
          }
          if (terminalEvents.has(payload.event)) {
            void refreshRun()
              .catch((error) => {
                console.error("[AgentMessageBubble] Failed to refresh completed run:", error);
              })
              .finally(() => {
                if (mounted) {
                  onComplete?.();
                }
              });
            es?.close();
          }
        });
      };

      [
        "run_queued",
        "run_started",
        "step_started",
        "step_done",
        "tool_called",
        "tool_done",
        "run_ready",
        "run_failed",
      ].forEach(bindRunEvent);

      es.addEventListener("stream_timeout" as never, () => {
        if (!mounted) return;
        void refreshRun().then((next) => {
          if (next && terminalStatuses.has(next.run.status)) {
            onComplete?.();
          }
        }).catch((error) => {
          console.error("[AgentMessageBubble] Failed after stream timeout:", error);
        });
        es?.close();
      });

      es.addEventListener("token" as never, (event: MessageEvent) => {
        if (!mounted || !event.data) return;
        setTokens((previous) => previous + String(event.data));
      });

      es.addEventListener("error" as never, (event: MessageEvent) => {
        const payload = typeof event.data === "string" ? event.data : "";
        if (payload.includes("AGENT_RUN_NOT_FOUND")) {
          setStatus("failed");
        }
        console.error("[AgentMessageBubble] SSE error", event);
        es?.close();
      });
    } catch (e) {
      console.error("[AgentMessageBubble] SSE setup error", e);
      if (mounted) setStatus("failed");
    }

    return () => {
      mounted = false;
      es?.close();
    };
  }, [initialResult?.run.status, onComplete, refreshRun, runId, token]);

  const timeline = useMemo(() => {
    if (events.length > 0) {
      return events;
    }
    return result?.trace.map((item, index) => ({
      sequence: index + 1,
      event: item.type,
      status: item.status,
      ref_type: item.type,
      ref_id: item.ref_id,
      name: item.name,
      at: item.at,
      metadata: item.metadata,
    })) ?? [];
  }, [events, result?.trace]);

  const citations: AgentCitation[] = result?.citations ?? [];
  const isTerminal = terminalStatuses.has(status);
  const isFailed = status === "failed";
  const answer = result?.run.summary || tokens;

  return (
    <View style={styles.bubble}>
      <View style={styles.header}>
        <View>
          <Text style={styles.agentName}>AI Agent</Text>
          {result?.run.goal ? <Text style={styles.goalText}>{result.run.goal}</Text> : null}
        </View>
        <View style={styles.headerRight}>
          {!isTerminal && <ActivityIndicator size="small" color="#2563eb" />}
          <Text style={isFailed ? styles.errorText : isTerminal ? styles.completedText : styles.runningText}>
            {statusLabel(status)}
          </Text>
        </View>
      </View>

      <TouchableOpacity onPress={() => setExpanded(!expanded)} style={styles.toggleRow}>
        <Text style={styles.toggleText}>
          {expanded ? "Hide run trace" : "Show run trace"} ({timeline.length} events)
        </Text>
      </TouchableOpacity>

      {expanded && timeline.length > 0 && (
        <View style={styles.stepsContainer}>
          {timeline.map((event) => (
            <View key={`${event.sequence}-${event.event}-${event.ref_id ?? "run"}`} style={styles.stepBox}>
              <Text style={styles.stepType}>{eventLabel(event)}</Text>
              <Text style={styles.stepOutput}>{statusLabel(event.status)} · {new Date(event.at).toLocaleTimeString()}</Text>
            </View>
          ))}
        </View>
      )}

      {answer ? (
        <View style={styles.tokenContainer}>
          <Text style={styles.tokenText}>{answer}</Text>
          {!isTerminal && tokens.length > 0 ? <Text style={styles.cursor}>|</Text> : null}
        </View>
      ) : null}

      {result?.run.next_step ? (
        <Text style={styles.nextStep}>Next step: {result.run.next_step}</Text>
      ) : null}

      {citations.length > 0 ? (
        <View style={styles.citationsContainer}>
          <Text style={styles.citationsTitle}>Evidence</Text>
          {citations.slice(0, 5).map((citation) => (
            <View key={`${citation.source_type}:${citation.source_id}`} style={styles.citationItem}>
              <Text style={styles.citationTitle}>{citation.title} · score {citation.score}</Text>
              <Text style={styles.citationSnippet}>{citation.snippet}</Text>
            </View>
          ))}
        </View>
      ) : null}
    </View>
  );
};

const styles = StyleSheet.create({
  bubble: {
    backgroundColor: "#f8fafc",
    borderColor: "#e2e8f0",
    borderWidth: 1,
    borderRadius: 8,
    padding: 12,
    marginVertical: 8,
    shadowColor: "#000",
    shadowOpacity: 0.05,
    shadowRadius: 5,
    shadowOffset: { width: 0, height: 2 },
    elevation: 2,
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 8,
    gap: 12,
  },
  agentName: {
    fontWeight: "700",
    fontSize: 15,
    color: "#0f172a",
  },
  goalText: {
    color: "#64748b",
    fontSize: 12,
    marginTop: 3,
  },
  headerRight: {
    flexDirection: "row",
    alignItems: "center",
    gap: 4,
  },
  completedText: {
    color: "#10b981",
    fontSize: 12,
    fontWeight: "600",
  },
  runningText: {
    color: "#2563eb",
    fontSize: 12,
    fontWeight: "600",
  },
  errorText: {
    color: "#ef4444",
    fontSize: 12,
    fontWeight: "600",
  },
  toggleRow: {
    paddingVertical: 6,
    borderBottomWidth: 1,
    borderBottomColor: "#f1f5f9",
    marginBottom: 6,
  },
  toggleText: {
    color: "#64748b",
    fontSize: 12,
    fontWeight: "600",
  },
  stepsContainer: {
    backgroundColor: "#f1f5f9",
    borderRadius: 8,
    padding: 8,
    marginBottom: 8,
  },
  stepBox: {
    marginBottom: 6,
  },
  stepType: {
    color: "#3b82f6",
    fontSize: 12,
    fontWeight: "700",
  },
  stepOutput: {
    color: "#475569",
    fontSize: 11,
    fontFamily: "Courier",
  },
  tokenContainer: {
    backgroundColor: "#ffffff",
    padding: 12,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: "#e2e8f0",
    flexDirection: "row",
    flexWrap: "wrap",
  },
  tokenText: {
    color: "#0f172a",
    fontSize: 14,
    lineHeight: 20,
  },
  cursor: {
    color: "#3b82f6",
    fontSize: 14,
    lineHeight: 20,
    marginLeft: 2,
  },
  nextStep: {
    color: "#1e293b",
    fontSize: 13,
    marginTop: 10,
    fontWeight: "600",
  },
  citationsContainer: {
    marginTop: 12,
    gap: 8,
  },
  citationsTitle: {
    color: "#0f172a",
    fontSize: 13,
    fontWeight: "700",
  },
  citationItem: {
    borderLeftWidth: 3,
    borderLeftColor: "#2563eb",
    paddingLeft: 10,
    paddingVertical: 4,
  },
  citationTitle: {
    color: "#334155",
    fontSize: 12,
    fontWeight: "700",
  },
  citationSnippet: {
    color: "#475569",
    fontSize: 12,
    marginTop: 3,
    lineHeight: 17,
  }
});
export default AgentMessageBubble;
