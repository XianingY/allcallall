import React, { useCallback, useEffect, useMemo, useState } from "react";
import {
  Alert,
  ScrollView,
  StyleSheet,
  Text,
  TouchableOpacity,
  View
} from "react-native";
import { NativeStackScreenProps } from "@react-navigation/native-stack";
import type { PurchasesOffering } from "react-native-purchases";

import { getRevenueCatConfig } from "../api/commercial";
import PrimaryButton from "../components/PrimaryButton";
import { useAuthContext } from "../context/AuthContext";
import { useCommercial } from "../context/CommercialContext";
import { RootStackParamList } from "../navigation/AppNavigator";
import BillingService from "../services/BillingService";

type Props = NativeStackScreenProps<RootStackParamList, "Subscription">;

const SubscriptionScreen: React.FC<Props> = () => {
  const { user } = useAuthContext();
  const { tier, usage, refreshCommercialState } = useCommercial();
  const [offering, setOffering] = useState<PurchasesOffering | null>(null);
  const [loading, setLoading] = useState(false);

  const usageSummary = useMemo(() => usage.find((item) => item.feature === "translation_minutes"), [usage]);

  const loadOfferings = useCallback(async () => {
    if (!user) {
      return;
    }
    try {
      setLoading(true);
      const initialized = await BillingService.initialize(`user:${user.id}`);
      if (!initialized) {
        setOffering(null);
        return;
      }
      const nextOffering = await BillingService.getOfferings();
      setOffering(nextOffering);
    } catch (error) {
      console.error("[SubscriptionScreen] Failed to load offerings:", error);
    } finally {
      setLoading(false);
    }
  }, [user]);

  useEffect(() => {
    void loadOfferings();
  }, [loadOfferings]);

  const handlePurchase = async (pkg: PurchasesOffering["availablePackages"][number]) => {
    try {
      setLoading(true);
      await BillingService.purchasePackage(pkg);
      await refreshCommercialState();
      Alert.alert("订阅成功", "Premium 权益已同步。");
    } catch (error) {
      console.error("[SubscriptionScreen] Purchase failed:", error);
      Alert.alert("购买失败", "无法完成购买，请稍后重试。");
    } finally {
      setLoading(false);
    }
  };

  const handleRestore = async () => {
    try {
      setLoading(true);
      await BillingService.restorePurchases();
      await refreshCommercialState();
      Alert.alert("恢复完成", "已尝试同步你的订阅状态。");
    } catch (error) {
      console.error("[SubscriptionScreen] Restore failed:", error);
      Alert.alert("恢复失败", "当前无法恢复购买。");
    } finally {
      setLoading(false);
    }
  };

  const handleManage = async () => {
    try {
      await BillingService.presentCustomerCenter();
    } catch (error) {
      console.error("[SubscriptionScreen] Customer center failed:", error);
      Alert.alert("暂不可用", "当前无法打开订阅管理。");
    }
  };

  const config = getRevenueCatConfig();

  return (
    <ScrollView style={styles.container} contentContainerStyle={styles.content}>
      <View style={styles.hero}>
        <Text style={styles.title}>Premium 订阅</Text>
        <Text style={styles.subtitle}>
          翻译是商用化主轴。基础通话永久免费，Premium 解锁无限实时翻译与更长历史保留。
        </Text>
      </View>

      <View style={styles.statusCard}>
        <Text style={styles.statusLabel}>当前权益</Text>
        <Text style={styles.statusValue}>{tier === "premium" ? "Premium" : "Free"}</Text>
        <Text style={styles.statusMeta}>
          {usageSummary
            ? usageSummary.unlimited
              ? "本月翻译不限量"
              : `本月翻译剩余 ${usageSummary.remaining_units} / ${usageSummary.limit_units} 分钟`
            : "正在同步翻译配额"}
        </Text>
      </View>

      <View style={styles.planCard}>
        <Text style={styles.planTitle}>Free</Text>
        <Text style={styles.planBullet}>无限基础 1:1 音视频和联系人</Text>
        <Text style={styles.planBullet}>每月 30 分钟实时翻译</Text>
        <Text style={styles.planBullet}>最近通话保留 30 天</Text>
      </View>

      <View style={[styles.planCard, styles.planPremium]}>
        <Text style={styles.planTitle}>Premium</Text>
        <Text style={styles.planBullet}>无限实时翻译</Text>
        <Text style={styles.planBullet}>高清画质档位</Text>
        <Text style={styles.planBullet}>最近通话保留 365 天</Text>
      </View>

      {config && offering?.availablePackages?.length ? (
        offering.availablePackages.map((pkg) => (
          <View key={pkg.identifier} style={styles.packageRow}>
            <View style={styles.packageInfo}>
              <Text style={styles.packageTitle}>{pkg.product.title}</Text>
              <Text style={styles.packageMeta}>{pkg.product.description || pkg.product.priceString}</Text>
            </View>
            <PrimaryButton
              title={pkg.product.priceString || "购买"}
              onPress={() => void handlePurchase(pkg)}
              disabled={loading}
            />
          </View>
        ))
      ) : (
        <View style={styles.placeholderCard}>
          <Text style={styles.placeholderTitle}>订阅产品尚未配置</Text>
          <Text style={styles.placeholderText}>
            需要设置 `EXPO_PUBLIC_REVENUECAT_API_KEY` 和产品 ID 才能在客户端发起购买。
          </Text>
        </View>
      )}

      <TouchableOpacity style={styles.secondaryButton} onPress={() => void handleRestore()}>
        <Text style={styles.secondaryButtonText}>恢复购买</Text>
      </TouchableOpacity>
      <TouchableOpacity style={styles.secondaryButton} onPress={() => void handleManage()}>
        <Text style={styles.secondaryButtonText}>管理订阅</Text>
      </TouchableOpacity>
    </ScrollView>
  );
};

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: "#eef2ff"
  },
  content: {
    padding: 18,
    paddingBottom: 40
  },
  hero: {
    marginBottom: 20
  },
  title: {
    fontSize: 30,
    fontWeight: "800",
    color: "#0f172a"
  },
  subtitle: {
    marginTop: 10,
    color: "#475569",
    lineHeight: 22
  },
  statusCard: {
    backgroundColor: "#fff",
    borderRadius: 20,
    padding: 20,
    marginBottom: 16
  },
  statusLabel: {
    color: "#64748b"
  },
  statusValue: {
    marginTop: 6,
    fontSize: 24,
    fontWeight: "800",
    color: "#0f172a"
  },
  statusMeta: {
    marginTop: 8,
    color: "#475569"
  },
  planCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 18,
    marginBottom: 12
  },
  planPremium: {
    backgroundColor: "#0f172a"
  },
  planTitle: {
    fontSize: 20,
    fontWeight: "800",
    color: "#0f172a",
    marginBottom: 10
  },
  planBullet: {
    color: "#475569",
    marginTop: 6
  },
  packageRow: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 16,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    marginBottom: 12
  },
  packageInfo: {
    flex: 1,
    paddingRight: 12
  },
  packageTitle: {
    fontWeight: "800",
    color: "#0f172a"
  },
  packageMeta: {
    marginTop: 6,
    color: "#64748b"
  },
  placeholderCard: {
    backgroundColor: "#fff",
    borderRadius: 18,
    padding: 16,
    marginBottom: 16
  },
  placeholderTitle: {
    fontWeight: "800",
    color: "#b45309"
  },
  placeholderText: {
    marginTop: 8,
    color: "#92400e"
  },
  secondaryButton: {
    marginTop: 10,
    alignItems: "center"
  },
  secondaryButtonText: {
    color: "#1d4ed8",
    fontWeight: "700"
  }
});

export default SubscriptionScreen;
