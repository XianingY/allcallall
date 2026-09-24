import { Pressable, ScrollView, Text, View } from "react-native";
import { StatusPill } from "./components";
import { compact } from "../agentDemoUtils";
import { styles } from "./styles";
import type { AgentLabController } from "./types";

  export const EvalTab = ({ controller }: { controller: AgentLabController }) => (
    <ScrollView style={styles.main} contentContainerStyle={styles.mainContent}>
      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Eval</Text>
        <View style={styles.evalGrid}>
          <View style={styles.evalCard}>
            <Text style={styles.rowTitle}>RAG quality</Text>
            <StatusPill status="pending" />
          </View>
          <View style={styles.evalCard}>
            <Text style={styles.rowTitle}>Workflow planner</Text>
            <StatusPill status="pending" />
          </View>
        </View>
      </View>
      <View style={styles.panel}>
        <Text style={styles.sectionTitle}>Recent Workflows</Text>
        {controller.workflows.map((item) => (
          <Pressable
            key={item.workflow.id}
            style={styles.rowItem}
            onPress={() => controller.setActiveWorkflow(item)}
          >
            <View style={styles.rowTop}>
              <Text style={styles.rowTitle}>Workflow #{item.workflow.id}</Text>
              <StatusPill status={item.workflow.status} />
            </View>
            <Text style={styles.rowMeta}>
              {compact(item.workflow.goal, 120)}
            </Text>
          </Pressable>
        ))}
      </View>
    </ScrollView>
  );
