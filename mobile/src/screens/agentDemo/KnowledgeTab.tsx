import { Platform, Pressable, ScrollView, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import TextField from "../../components/TextField";
import { StatusPill, WebFilePicker } from "./components";
import { compact, formatTime } from "../agentDemoUtils";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const KnowledgeTab = ({ controller }: { controller: AgentLabController }) => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <Text style={styles.pageSectionTitle}>Ingest</Text>
      <View style={styles.grid}>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>Manual Text</Text>
          <TextField
            label="Title"
            value={controller.manualTitle}
            onChangeText={controller.setManualTitle}
          />
          <TextField
            label="Text"
            value={controller.manualText}
            onChangeText={controller.setManualText}
            multiline
            style={styles.textArea}
          />
          <PrimaryButton
            title="Add Text"
            onPress={controller.handleCreateManualSource}
            disabled={controller.busy || !controller.manualText.trim()}
            style={styles.fullButton}
          />
        </View>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>URL</Text>
          <TextField
            label="Title"
            value={controller.urlTitle}
            onChangeText={controller.setURLTitle}
          />
          <TextField
            label="URL"
            value={controller.urlValue}
            onChangeText={controller.setURLValue}
            autoCapitalize="none"
          />
          <PrimaryButton
            title="Add URL"
            onPress={controller.handleCreateURLSource}
            disabled={controller.busy || !controller.urlValue.trim()}
            style={styles.fullButton}
          />
        </View>
        <View style={[styles.panel, styles.formPanel]}>
          <Text style={styles.sectionTitle}>File Upload</Text>
          <TextField
            label="Title"
            value={controller.fileTitle}
            onChangeText={controller.setFileTitle}
          />
          {Platform.OS === "web" ? (
            <WebFilePicker onFile={controller.handleFileSelected} />
          ) : (
            <Text style={styles.emptyText}>
              File upload is available in the web build.
            </Text>
          )}
        </View>
      </View>

      <Text style={styles.pageSectionTitle}>Sources</Text>
      <View style={styles.grid}>
        <View style={[styles.panel, styles.listPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Sources</Text>
            <Text style={styles.contextLine}>{controller.sources.length}</Text>
          </View>
          {controller.sources.map((source) => (
            <Pressable
              key={source.id}
              style={styles.rowItem}
              onPress={() => void controller.selectSource(source.id)}
            >
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{source.title}</Text>
                <StatusPill status={source.status} />
              </View>
              <Text style={styles.rowMeta}>
                {source.kind} · group {source.source_group_id ?? "-"} ·{" "}
                {source.dedupe_status || "unique"} ·{" "}
                {formatTime(source.updated_at)}
              </Text>
              <Text style={styles.rowMeta}>
                active version #{source.active_version_id ?? "-"} · authority{" "}
                {Math.round((source.authority_score ?? 0) * 100)}
                {source.conversation_id
                  ? ` · conversation #${source.conversation_id}`
                  : " · organization"}
              </Text>
              {source.last_error ? (
                <Text style={styles.errorText}>
                  {compact(source.last_error)}
                </Text>
              ) : null}
              <View style={styles.inlineActions}>
                <Pressable
                  style={styles.inlineButton}
                  onPress={() =>
                    void controller.reingestKnowledgeSource(controller.token ?? "", source.id).then(
                      controller.refreshKnowledge,
                    )
                  }
                >
                  <Text style={styles.inlineButtonText}>Reingest</Text>
                </Pressable>
              </View>
            </Pressable>
          ))}
          {controller.sources.length === 0 ? (
            <Text style={styles.emptyText}>No sources yet.</Text>
          ) : null}
        </View>

        <View style={[styles.panel, styles.previewPanel]}>
          <Text style={styles.sectionTitle}>Source Preview</Text>
          {controller.sourceDetail ? (
            <>
              <Text style={styles.previewTitle}>
                {controller.sourceDetail.source.title}
              </Text>
              <Text style={styles.rowMeta}>
                {controller.sourceDetail.source.kind} ·{" "}
                {controller.sourceDetail.versions[0]?.chunk_count ?? 0} chunks
              </Text>
              <View style={styles.statsRow}>
                <Text style={styles.statChip}>
                  Indexed {controller.selectedChunkStats.indexed}
                </Text>
                <Text style={styles.statChip}>
                  Failed {controller.selectedChunkStats.failed}
                </Text>
                <Text style={styles.statChip}>
                  Skipped {controller.selectedChunkStats.skipped}
                </Text>
                <Text style={styles.statChip}>
                  Pending {controller.selectedChunkStats.pending}
                </Text>
              </View>
              {controller.sourceDetail.versions.map((version) => (
                <View key={version.id} style={styles.versionRow}>
                  <Text style={styles.rowTitle}>Version {version.version}</Text>
                  <StatusPill status={version.status} />
                </View>
              ))}
              {controller.sourceDetail.chunks.map((chunk) => (
                <View key={chunk.id} style={styles.chunkBox}>
                  <View style={styles.rowTop}>
                    <Text style={styles.chunkTitle}>
                      Chunk {chunk.chunk_index}
                    </Text>
                    <StatusPill status={chunk.index_status} />
                  </View>
                  <Text style={styles.citationSnippet}>{chunk.snippet}</Text>
                </View>
              ))}
            </>
          ) : (
            <Text style={styles.emptyText}>Select a source.</Text>
          )}
        </View>
      </View>

      <Text style={styles.pageSectionTitle}>Duplicate Review</Text>
      <View style={styles.grid}>
        <View style={[styles.panel, styles.listPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Source Groups</Text>
            <Text style={styles.contextLine}>{controller.sourceGroups.length}</Text>
          </View>
          {controller.sourceGroups.map((group) => (
            <View key={group.id} style={styles.rowItem}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>{group.title}</Text>
                <StatusPill status={group.status} />
              </View>
              <Text style={styles.rowMeta}>
                canonical #{group.canonical_source_id ?? "-"} · authority{" "}
                {Math.round((group.authority_score ?? 0) * 100)}
              </Text>
              {controller.sources
                .filter((source) => source.source_group_id === group.id)
                .map((source) => (
                  <View key={source.id} style={styles.inlineRow}>
                    <Text style={styles.rowMeta}>
                      #{source.id} {source.title}
                    </Text>
                    {group.canonical_source_id !== source.id ? (
                      <Pressable
                        style={styles.inlineButton}
                        onPress={() =>
                          void controller.handleSetCanonical(group.id, source.id)
                        }
                      >
                        <Text style={styles.inlineButtonText}>
                          Make canonical
                        </Text>
                      </Pressable>
                    ) : null}
                  </View>
                ))}
            </View>
          ))}
          {controller.sourceGroups.length === 0 ? (
            <Text style={styles.emptyText}>No source groups.</Text>
          ) : null}
        </View>

        <View style={[styles.panel, styles.previewPanel]}>
          <View style={styles.panelHeader}>
            <Text style={styles.sectionTitle}>Duplicate Review</Text>
            <Text style={styles.contextLine}>{controller.duplicateCandidates.length}</Text>
          </View>
          {controller.duplicateCandidates.map((item) => (
            <View key={item.id} style={styles.rowItem}>
              <View style={styles.rowTop}>
                <Text style={styles.rowTitle}>
                  {controller.sourceTitleById.get(item.source_id) ||
                    `Source #${item.source_id}`}
                </Text>
                <StatusPill status={item.status} />
              </View>
              <Text style={styles.rowMeta}>
                Candidate{" "}
                {controller.sourceTitleById.get(item.candidate_source_id) ||
                  `#${item.candidate_source_id}`}{" "}
                · {item.duplicate_kind} · similarity{" "}
                {Math.round(item.similarity * 100)} · group{" "}
                {item.source_group_id ?? "-"}
              </Text>
              {item.status === "pending" ? (
                <View style={styles.inlineActions}>
                  <Pressable
                    style={styles.approveButton}
                    onPress={() =>
                      void controller.handleDuplicateDecision(item.id, "confirm")
                    }
                  >
                    <Text style={styles.approveButtonText}>Confirm</Text>
                  </Pressable>
                  <Pressable
                    style={styles.rejectButton}
                    onPress={() =>
                      void controller.handleDuplicateDecision(item.id, "reject")
                    }
                  >
                    <Text style={styles.rejectButtonText}>Reject</Text>
                  </Pressable>
                </View>
              ) : null}
            </View>
          ))}
          {controller.duplicateCandidates.length === 0 ? (
            <Text style={styles.emptyText}>No duplicate candidates.</Text>
          ) : null}
        </View>
      </View>

      <Text style={styles.pageSectionTitle}>Recovery</Text>
      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Dead Letters</Text>
        {controller.deadLetters.map((item) => (
          <View key={item.id} style={styles.rowItem}>
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>{item.event}</Text>
              <StatusPill status={item.status} />
            </View>
            <Text style={styles.rowMeta}>
              attempts {item.attempts} · #{item.id}
              {item.available_at
                ? ` · available ${formatTime(item.available_at)}`
                : ""}
            </Text>
            {item.last_error ? (
              <Text style={styles.errorText}>
                {compact(item.last_error, 260)}
              </Text>
            ) : null}
            <Pressable
              style={styles.inlineButton}
              onPress={() =>
                void controller.retryKnowledgeDeadLetter(controller.token ?? "", item.id).then(
                  controller.refreshKnowledge,
                )
              }
            >
              <Text style={styles.inlineButtonText}>Retry</Text>
            </Pressable>
          </View>
        ))}
        {controller.deadLetters.length === 0 ? (
          <Text style={styles.emptyText}>No failed RAG events.</Text>
        ) : null}
      </View>
    </ScrollView>
  );
