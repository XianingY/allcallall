import { Pressable, ScrollView, Text, View } from "react-native";
import PrimaryButton from "../../components/PrimaryButton";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const AgentLabSidebar = ({ controller }: { controller: AgentLabController }) => (
    <View style={controller.isWide ? styles.sidebar : styles.panel}>
      <Text style={styles.eyebrow}>{controller.currentOrganization?.name}</Text>
      <Text style={styles.heading}>
        {controller.knowledgeOnly ? "Knowledge Center" : "Agent Lab"}
      </Text>
      {!controller.knowledgeOnly ? (
        <View style={styles.tabRow}>
          {controller.visibleTabs.map((tab) => (
            <Pressable
              key={tab.key}
              style={[
                styles.tabButton,
                controller.activeTab === tab.key && styles.tabButtonActive,
              ]}
              onPress={() => controller.setActiveTab(tab.key)}
            >
              <Text
                style={[
                  styles.tabButtonText,
                  controller.activeTab === tab.key && styles.tabButtonTextActive,
                ]}
              >
                {tab.label}
              </Text>
            </Pressable>
          ))}
        </View>
      ) : (
        <Text style={styles.contextLine}>
          Manage ingestion, duplicate review, canonical sources, and retry
          queues.
        </Text>
      )}
      <View style={styles.actionRow}>
        <PrimaryButton
          title={controller.busy ? "Working..." : "Refresh"}
          onPress={() => void controller.refreshAll()}
          disabled={controller.busy}
          style={styles.actionButton}
        />
        <PrimaryButton
          title={controller.knowledgeOnly ? "Open Lab" : "Demo"}
          onPress={
            controller.knowledgeOnly
              ? () => controller.navigation.navigate("AgentDemo")
              : controller.handleCreateDemoThread
          }
          disabled={controller.busy}
          style={styles.secondaryButton}
        />
      </View>
      {controller.notice ? <Text style={styles.notice}>{controller.notice}</Text> : null}
      <ScrollView
        style={styles.threadList}
        contentContainerStyle={styles.threadListContent}
      >
        {controller.conversations.map((conversation) => {
          const selected = conversation.id === controller.selectedConversationId;
          return (
            <Pressable
              key={conversation.id}
              style={[styles.threadItem, selected && styles.threadItemActive]}
              onPress={() => controller.setSelectedConversationId(conversation.id)}
            >
              <Text
                style={[
                  styles.threadTitle,
                  selected && styles.threadTitleActive,
                ]}
                numberOfLines={1}
              >
                {conversation.title || conversation.type}
              </Text>
              <Text
                style={[styles.threadMeta, selected && styles.threadMetaActive]}
              >
                {conversation.priority.toUpperCase()} ·{" "}
                {conversation.status.toUpperCase()}
              </Text>
              <Text
                style={[
                  styles.threadPreview,
                  selected && styles.threadPreviewActive,
                ]}
                numberOfLines={2}
              >
                {conversation.last_message_preview || "No messages"}
              </Text>
            </Pressable>
          );
        })}
      </ScrollView>
    </View>
  );
