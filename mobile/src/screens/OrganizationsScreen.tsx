import React, { useState } from "react";
import { Alert, FlatList, StyleSheet, Text, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";

type Props = NativeStackScreenProps<RootStackParamList, "Organizations">;

const OrganizationsScreen: React.FC<Props> = () => {
  const {
    organizations,
    currentOrganization,
    loading,
    selectOrganization,
    createWorkspace
  } = useOrganization();
  const [name, setName] = useState("");
  const [submitting, setSubmitting] = useState(false);

  const handleCreate = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      return;
    }
    try {
      setSubmitting(true);
      await createWorkspace(trimmed);
      setName("");
    } catch (error) {
      console.error("[OrganizationsScreen] Failed to create workspace:", error);
      Alert.alert("创建失败", "无法创建工作区，请稍后再试。");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.title}>当前工作区</Text>
      <Text style={styles.currentName}>
        {currentOrganization?.name ?? "未选择工作区"}
      </Text>

      <TextField
        label="新建工作区"
        value={name}
        onChangeText={setName}
        placeholder="输入工作区名称"
      />
      <PrimaryButton
        title={submitting ? "创建中..." : "创建工作区"}
        onPress={handleCreate}
        disabled={submitting || !name.trim()}
        style={styles.createButton}
      />

      <FlatList
        data={organizations}
        keyExtractor={(item) => String(item.id)}
        refreshing={loading}
        renderItem={({ item }) => {
          const isCurrent = item.id === currentOrganization?.id;
          return (
            <View style={[styles.card, isCurrent && styles.cardCurrent]}>
              <View style={styles.cardHeader}>
                <Text style={styles.cardTitle}>{item.name}</Text>
                <Text style={styles.role}>{item.role}</Text>
              </View>
              <Text style={styles.slug}>{item.slug}</Text>
              <PrimaryButton
                title={isCurrent ? "当前工作区" : "切换到此"}
                onPress={() => void selectOrganization(item.id)}
                disabled={isCurrent}
                style={styles.switchButton}
              />
            </View>
          );
        }}
      />
    </View>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#f8fafc",
    padding: 16
  },
  title: {
    fontSize: 14,
    color: "#64748b"
  },
  currentName: {
    fontSize: 24,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 16
  },
  createButton: {
    marginBottom: 16
  },
  card: {
    backgroundColor: "#fff",
    borderRadius: 16,
    padding: 16,
    marginBottom: 12,
    borderWidth: 1,
    borderColor: "#e2e8f0"
  },
  cardCurrent: {
    borderColor: "#2563eb"
  },
  cardHeader: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center"
  },
  cardTitle: {
    fontSize: 18,
    fontWeight: "600",
    color: "#0f172a"
  },
  role: {
    color: "#2563eb",
    fontWeight: "600"
  },
  slug: {
    marginTop: 6,
    color: "#64748b",
    marginBottom: 12
  },
  switchButton: {
    marginTop: 4
  }
});

export default OrganizationsScreen;
