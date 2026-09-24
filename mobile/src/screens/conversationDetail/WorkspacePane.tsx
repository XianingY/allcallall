import { Pressable, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import { styles } from "./styles";
import type { WorkspacePaneProps } from "./types";
import {
  STATUS_OPTIONS,
  PRIORITY_OPTIONS,
  MEETING_PRESETS,
  citationModeLabel,
  approvalPreview,
} from "../conversationDetailUtils";

const WorkspacePane = ({
  conversation,
  workspace,
  latestRoom,
  latestFollowup,
  assigneeLabel,
  boundContact,
  agentContext,
  contacts,
  notes,
  latestRecording,
  activeWorkflow,
  pendingApprovals,
  completedTaskCount,
  executedApprovalCount,
  rejectedApprovalCount,
  transcriptStatusText,
  agentStatusLabel,
  meetingTranscriptReady,
  workflowLoading,
  navigation,
  onCopyLink,
  onCreateMeeting,
  onAssignSelf,
  onUnassign,
  onRunMeetingAgent,
  onOpenWorkflowDebug,
  onCitationPress,
  onApprovalDecision,
  onUpdateStatus,
  onUpdatePriority,
  onBindContact,
  onDownloadRecording,
  noteDraft,
  onNoteDraftChange,
  onAddNote,
}: WorkspacePaneProps) => (
  <>
    <Text style={styles.heading}>{conversation.title || "协作线程"}</Text>

    <View style={styles.summaryCard}>
      <Text style={styles.summaryText}>
        负责人 {workspace?.assignee_label || assigneeLabel}
      </Text>
      <Text style={styles.summaryText}>
        状态 {workspace?.status || conversation.status}
      </Text>
      <Text style={styles.summaryText}>
        优先级 {workspace?.priority || conversation.priority}
      </Text>
      <Text style={styles.summaryText}>
        关联联系人{" "}
        {boundContact?.display_name || boundContact?.email || "未绑定"}
      </Text>
      <PrimaryButton
        title="复制线程 Web 链接"
        onPress={() => void onCopyLink()}
        style={styles.inlineButtonSecondary}
      />
      {latestRoom ? (
        <PrimaryButton
          title="进入当前会议"
          onPress={() =>
            navigation.navigate("PreJoin", {
              roomId: latestRoom?.id ?? 0,
              title: latestRoom?.title ?? "Meeting",
              conversationId: latestRoom?.conversation_id ?? null,
              joinOptions: {
                audioEnabled: true,
                videoEnabled: true,
                cameraFacing: "front",
                speakerOn: true,
              },
            })
          }
          style={styles.inlineButton}
        />
      ) : (
        <PrimaryButton
          title="升级为会议"
          onPress={onCreateMeeting}
          style={styles.inlineButton}
        />
      )}
    </View>

    <View style={styles.buttonRow}>
      <PrimaryButton
        title="指派给我"
        onPress={onAssignSelf}
        style={styles.button}
      />
      <PrimaryButton
        title="清空负责人"
        onPress={onUnassign}
        style={styles.buttonSecondary}
      />
    </View>

    <View style={styles.infoCard}>
      <View style={styles.agentHeader}>
        <View>
          <Text style={styles.infoTitle}>Meeting Agent</Text>
          <Text style={styles.infoMeta}>{transcriptStatusText}</Text>
        </View>
        <View style={styles.agentStatusBadge}>
          <Text style={styles.agentStatusText}>{agentStatusLabel}</Text>
        </View>
      </View>
      <View style={styles.agentContextGrid}>
        <Text style={styles.agentContextItem}>
          Call {agentContext?.latest_call_id || "-"}
        </Text>
        <Text style={styles.agentContextItem}>
          Knowledge {agentContext?.knowledge_source_count ?? 0}
        </Text>
        <Text style={styles.agentContextItem}>
          Approvals{" "}
          {agentContext?.pending_approval_count ?? pendingApprovals.length}
        </Text>
        <Text style={styles.agentContextItem}>
          Workflow {agentContext?.last_workflow_id ?? activeWorkflow?.workflow.id ?? "-"}
        </Text>
      </View>
      {agentContext?.latest_transcript_at ? (
        <Text style={styles.infoMeta}>
          Latest transcript{" "}
          {new Date(agentContext.latest_transcript_at).toLocaleString()}
        </Text>
      ) : null}
      {agentContext?.last_agent_run_at ? (
        <Text style={styles.infoMeta}>
          Last workflow {agentContext.last_agent_status || "-"} ·{" "}
          {agentContext.last_workflow_preset || activeWorkflow?.workflow.preset || "custom"} ·{" "}
          {new Date(agentContext.last_agent_run_at).toLocaleString()}
        </Text>
      ) : null}
      {activeWorkflow ? (
        <Text style={styles.infoMeta}>
          Progress {completedTaskCount}/{activeWorkflow.tasks.length} tasks ·
          write-back executed {executedApprovalCount}
          {rejectedApprovalCount ? ` · rejected ${rejectedApprovalCount}` : ""}
        </Text>
      ) : null}
      {agentContext?.latest_memory_keys?.length ? (
        <View style={styles.memoryChipRow}>
          {agentContext.latest_memory_keys.map((key) => (
            <Text key={key} style={styles.memoryChip}>
              {key}
            </Text>
          ))}
        </View>
      ) : null}
      <View style={styles.optionRow}>
        {MEETING_PRESETS.map((preset) => (
          <PrimaryButton
            key={preset.key}
            title={preset.label}
            onPress={() => void onRunMeetingAgent({ preset: preset.key })}
            disabled={
              workflowLoading ||
              (preset.key === "meeting_brief" && !meetingTranscriptReady)
            }
            style={styles.option}
          />
        ))}
      </View>
      <View style={styles.buttonRow}>
        <PrimaryButton
          title="Knowledge Center"
          onPress={() => navigation.navigate("KnowledgeCenter")}
          style={styles.button}
        />
        <PrimaryButton
          title="Workflow Debug"
          onPress={onOpenWorkflowDebug}
          disabled={!activeWorkflow}
          style={styles.buttonSecondary}
        />
      </View>
      {activeWorkflow?.workflow?.summary ? (
        <View style={styles.agentResultBox}>
          <Text style={styles.citationTitle}>会议摘要</Text>
          <Text style={styles.infoBody}>{activeWorkflow.workflow.summary}</Text>
        </View>
      ) : (
        <Text style={styles.infoMeta}>
          基于 final transcript、follow-up、memory 和线程上下文生成 grounded
          结果。
        </Text>
      )}
      {activeWorkflow?.workflow?.next_step ? (
        <Text style={styles.infoMeta}>
          下一步 {activeWorkflow.workflow.next_step}
        </Text>
      ) : null}
      {activeWorkflow?.workflow?.action_items?.length ? (
        <Text style={styles.infoMeta}>
          行动项 {activeWorkflow.workflow.action_items.join(" / ")}
        </Text>
      ) : null}
      {activeWorkflow?.workflow?.risk_flags?.length ? (
        <Text style={styles.infoMeta}>
          风险点 {activeWorkflow.workflow.risk_flags.join(" / ")}
        </Text>
      ) : null}
      {activeWorkflow?.citations?.length ? (
        <View style={styles.citationList}>
          {activeWorkflow.citations.slice(0, 4).map((citation, index) => (
            <Pressable
              key={`${citation.source_type}:${citation.source_id}:${index}`}
              style={styles.citationItem}
              onPress={() => void onCitationPress(citation)}
            >
              <View style={styles.citationHeader}>
                <Text style={styles.citationTitle}>{citation.title}</Text>
                <Text style={styles.citationBadge}>
                  {citationModeLabel(citation.retrieval_mode)}
                </Text>
              </View>
              <Text style={styles.citationMeta}>
                {citation.source_type} · score {citation.score}
              </Text>
              <Text style={styles.citationSnippet}>{citation.snippet}</Text>
            </Pressable>
          ))}
        </View>
      ) : null}
      {pendingApprovals.length ? (
        <View style={styles.citationList}>
          <Text style={styles.infoMeta}>
            Pending approvals{" "}
            {pendingApprovals.map((item) => item.tool_name).join(" / ")}
          </Text>
          {pendingApprovals.map((approval) => (
            <View key={approval.id} style={styles.approvalItem}>
              <Text style={styles.citationTitle}>
                {approvalPreview(approval).title}
              </Text>
              {approvalPreview(approval).lines.map((line) => (
                <Text key={line} style={styles.citationSnippet}>
                  {line}
                </Text>
              ))}
              {approval.mcp_revision_id && approval.mcp_revision_id > 0 ? (
                <Text style={styles.citationMeta}>
                  {approval.mcp_installation_id
                    ? `MCP Installation #${approval.mcp_installation_id}`
                    : "MCP"}{" "}
                  · Revision #{approval.mcp_revision_id}
                </Text>
              ) : null}
              <Text style={styles.citationMeta}>
                Schema {approval.tool_schema_version || "-"}
                {approval.approval_request_id
                  ? ` · Approval ${approval.approval_request_id} · checkpoint v${approval.approval_checkpoint_version}`
                  : ""}
              </Text>
              <View style={styles.inlineActionRow}>
                <Pressable
                  style={styles.approveChip}
                  onPress={() => void onApprovalDecision(approval, "approve")}
                >
                  <Text style={styles.approveChipText}>Approve</Text>
                </Pressable>
                <Pressable
                  style={styles.rejectChip}
                  onPress={() => void onApprovalDecision(approval, "reject")}
                >
                  <Text style={styles.rejectChipText}>Reject</Text>
                </Pressable>
              </View>
            </View>
          ))}
        </View>
      ) : null}
    </View>

    <Text style={styles.sectionTitle}>状态</Text>
    <View style={styles.optionRow}>
      {STATUS_OPTIONS.map((status) => (
        <PrimaryButton
          key={status}
          title={status}
          onPress={() => void onUpdateStatus(status)}
          style={
            conversation.status === status
              ? styles.optionActive
              : styles.option
          }
        />
      ))}
    </View>

    <Text style={styles.sectionTitle}>优先级</Text>
    <View style={styles.optionRow}>
      {PRIORITY_OPTIONS.map((priority) => (
        <PrimaryButton
          key={priority}
          title={priority}
          onPress={() => void onUpdatePriority(priority)}
          style={
            conversation.priority === priority
              ? styles.optionActive
              : styles.option
          }
        />
      ))}
    </View>

    <Text style={styles.sectionTitle}>联系人绑定</Text>
    {boundContact ? (
      <View style={styles.infoCard}>
        <Text style={styles.infoTitle}>
          {boundContact.display_name || boundContact.email}
        </Text>
        <Text style={styles.infoMeta}>{boundContact.email}</Text>
        {boundContact.profile?.company ? (
          <Text style={styles.infoMeta}>
            公司 {boundContact.profile.company}
          </Text>
        ) : null}
        {boundContact.profile?.role ? (
          <Text style={styles.infoMeta}>
            角色 {boundContact.profile.role}
          </Text>
        ) : null}
        {boundContact.profile?.timezone ? (
          <Text style={styles.infoMeta}>
            时区 {boundContact.profile.timezone}
          </Text>
        ) : null}
        {boundContact.profile?.default_source_lang ||
        boundContact.profile?.default_target_lang ? (
          <Text style={styles.infoMeta}>
            默认语言 {boundContact.profile?.default_source_lang || "-"} →{" "}
            {boundContact.profile?.default_target_lang || "-"}
          </Text>
        ) : null}
        <View style={styles.buttonRow}>
          <PrimaryButton
            title="查看联系人"
            onPress={() =>
              navigation.navigate("ContactDetail", { contact: boundContact })
            }
            style={styles.button}
          />
          <PrimaryButton
            title="解除绑定"
            onPress={() => void onBindContact(null)}
            style={styles.buttonSecondary}
          />
        </View>
      </View>
    ) : (
      <View style={styles.contactList}>
        {contacts.slice(0, 4).map((contact) => (
          <PrimaryButton
            key={contact.id}
            title={`绑定 ${contact.display_name || contact.email}`}
            onPress={() => void onBindContact(contact.id)}
            style={styles.contactButton}
          />
        ))}
      </View>
    )}

    {workspace ? (
      <View style={styles.infoCard}>
        <Text style={styles.infoTitle}>顶部工作区</Text>
        {workspace.latest_meeting ? (
          <Text style={styles.infoMeta}>
            最近会议 {workspace.latest_meeting.title}
          </Text>
        ) : null}
        {workspace.latest_recording ? (
          <Text style={styles.infoMeta}>
            最近录音资产 #{workspace.latest_recording.session.id}
            {workspace.latest_recording.transcription
              ? ` · 转写 ${workspace.latest_recording.transcription.status}`
              : ""}
          </Text>
        ) : null}
        {workspace.meeting_summary?.summary ? (
          <Text style={styles.infoBody}>
            {workspace.meeting_summary.summary}
          </Text>
        ) : null}
        {workspace.meeting_summary?.action_items?.length ? (
          <Text style={styles.infoMeta}>
            Action items{" "}
            {workspace.meeting_summary.action_items.join(" / ")}
          </Text>
        ) : null}
        {workspace.meeting_summary?.next_step ? (
          <Text style={styles.infoMeta}>
            Next step {workspace.meeting_summary.next_step}
          </Text>
        ) : null}
        {workspace.latest_note ? (
          <Text style={styles.infoMeta}>
            最近备注{" "}
            {workspace.latest_note.author_display_name ||
              workspace.latest_note.author_email}{" "}
            ·{" "}
            {new Date(workspace.latest_note.created_at).toLocaleString()}
          </Text>
        ) : null}
      </View>
    ) : null}

    {notes.length > 0 ? (
      <View style={styles.infoCard}>
        <Text style={styles.infoTitle}>最近三条内部备注</Text>
        {notes.slice(0, 3).map((note) => (
          <View key={note.id} style={styles.noteRow}>
            <Text style={styles.noteBody}>{note.body}</Text>
            <Text style={styles.infoMeta}>
              {note.author_display_name || note.author_email} ·{" "}
              {new Date(note.created_at).toLocaleString()}
            </Text>
          </View>
        ))}
      </View>
    ) : null}

    {latestFollowup ? (
      <View style={styles.infoCard}>
        <Text style={styles.infoTitle}>最近会议/通话摘要</Text>
        <Text style={styles.infoBody}>
          {workspace?.meeting_summary?.summary ||
            latestFollowup.summary_cn ||
            latestFollowup.summary_en ||
            "暂无摘要"}
        </Text>
        {workspace?.meeting_summary?.next_step || latestFollowup.next_step ? (
          <Text style={styles.infoMeta}>
            下一步{" "}
            {workspace?.meeting_summary?.next_step || latestFollowup.next_step}
          </Text>
        ) : null}
      </View>
    ) : null}

    {latestRecording ? (
      <View style={styles.infoCard}>
        <Text style={styles.infoTitle}>最近录音资产</Text>
        <Text style={styles.infoMeta}>
          录音会话 #{latestRecording.session.id}
        </Text>
        <Text style={styles.infoMeta}>
          状态 {latestRecording.session.status}
        </Text>
        <Text style={styles.infoMeta}>
          文件数 {latestRecording.files.length}
        </Text>
        <Text style={styles.infoMeta}>
          转写 {latestRecording.transcription?.status ?? "not_requested"}
          {latestRecording.transcription?.segment_count
            ? ` · ${latestRecording.transcription.segment_count} segments`
            : ""}
        </Text>
        {latestRecording.transcription ? (
          <PrimaryButton
            title="查看会议转写"
            onPress={() =>
              navigation.navigate("RecordingTranscript", {
                recordingId: latestRecording.session.id,
              })
            }
            style={styles.recordingButton}
          />
        ) : null}
        {latestRecording.files.slice(0, 2).map((file) => (
          <View key={file.id} style={styles.recordingFileRow}>
            <Text style={styles.recordingFileTitle}>{file.file_name}</Text>
            <Text style={styles.infoMeta}>
              {file.recording_kind} · {file.file_size_bytes} bytes ·{" "}
              {file.duration_seconds}s
            </Text>
            <PrimaryButton
              title="下载最近录音"
              onPress={() =>
                void onDownloadRecording(
                  latestRecording.session.id,
                  file.id,
                  file.file_name,
                )
              }
              style={styles.recordingButton}
            />
          </View>
        ))}
        <PrimaryButton
          title="查看全部录音资产"
          onPress={() => navigation.navigate("Recordings")}
          style={styles.recordingLinkButton}
        />
      </View>
    ) : null}

    <TextField
      label="内部备注"
      value={noteDraft}
      onChangeText={onNoteDraftChange}
      placeholder="记录交接说明、风险点或下一步动作"
    />
    <PrimaryButton
      title="添加内部备注"
      onPress={onAddNote}
      disabled={!noteDraft.trim()}
      style={styles.createNoteButton}
    />
  </>
);

export default WorkspacePane;
