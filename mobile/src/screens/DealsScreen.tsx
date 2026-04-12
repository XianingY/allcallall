import React, { useCallback, useEffect, useState } from "react";
import { Alert, FlatList, StyleSheet, Text, TouchableOpacity, View } from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";

import { createDeal, listDeals, type DealRecord } from "../api/collaboration";
import { useAuthContext } from "../context/AuthContext";
import { useOrganization } from "../context/OrganizationContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import PrimaryButton from "../components/PrimaryButton";
import TextField from "../components/TextField";

type Props = NativeStackScreenProps<RootStackParamList, "Deals">;

const DealsScreen: React.FC<Props> = ({ navigation }) => {
  const { token } = useAuthContext();
  const { currentOrganization } = useOrganization();
  const [items, setItems] = useState<DealRecord[]>([]);
  const [loading, setLoading] = useState(false);
  const [title, setTitle] = useState("");

  const loadDeals = useCallback(async () => {
    if (!token || !currentOrganization) {
      setItems([]);
      return;
    }
    try {
      setLoading(true);
      const data = await listDeals(token);
      setItems(data);
    } catch (error) {
      console.error("[DealsScreen] Failed to load deals:", error);
      Alert.alert("加载失败", "无法加载商机列表。");
    } finally {
      setLoading(false);
    }
  }, [currentOrganization, token]);

  useEffect(() => {
    void loadDeals();
  }, [loadDeals]);

  const handleCreateDeal = async () => {
    if (!token || !title.trim()) {
      return;
    }
    try {
      const deal = await createDeal(token, { title: title.trim() });
      setTitle("");
      await loadDeals();
      navigation.navigate("DealDetail", { deal });
    } catch (error) {
      console.error("[DealsScreen] Failed to create deal:", error);
      Alert.alert("创建失败", "无法创建商机。");
    }
  };

  return (
    <View style={styles.container}>
      <Text style={styles.heading}>
        {currentOrganization?.name ?? "当前工作区"} 商机
      </Text>
      <TextField
        label="新增商机"
        value={title}
        onChangeText={setTitle}
        placeholder="例如：Tokyo Distributor Expansion"
      />
      <PrimaryButton title="创建商机" onPress={handleCreateDeal} disabled={!title.trim()} style={styles.createButton} />

      <FlatList
        data={items}
        keyExtractor={(item) => String(item.id)}
        refreshing={loading}
        onRefresh={() => void loadDeals()}
        renderItem={({ item }) => (
          <TouchableOpacity style={styles.card} onPress={() => navigation.navigate("DealDetail", { deal: item })}>
            <Text style={styles.title}>{item.title}</Text>
            <Text style={styles.stage}>{item.stage_name || item.status}</Text>
            <Text style={styles.meta}>
              {item.currency} {(item.value_cents / 100).toFixed(2)}
            </Text>
          </TouchableOpacity>
        )}
        ListEmptyComponent={
          <View style={styles.empty}>
            <Text style={styles.emptyText}>当前还没有商机。</Text>
          </View>
        }
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
  heading: {
    fontSize: 22,
    fontWeight: "700",
    color: "#0f172a",
    marginBottom: 12
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
  title: {
    fontSize: 17,
    fontWeight: "600",
    color: "#0f172a"
  },
  stage: {
    color: "#2563eb",
    marginTop: 6
  },
  meta: {
    color: "#64748b",
    marginTop: 8
  },
  empty: {
    alignItems: "center",
    paddingTop: 48
  },
  emptyText: {
    color: "#64748b"
  }
});

export default DealsScreen;
