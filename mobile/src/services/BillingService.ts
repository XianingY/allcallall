import Purchases, {
  type CustomerInfo,
  type PurchasesOffering,
  LOG_LEVEL
} from "react-native-purchases";
import { Linking, Platform } from "react-native";
import RevenueCatUI from "react-native-purchases-ui";

import { getRevenueCatConfig } from "../api/commercial";

class BillingService {
  private configured = false;

  async initialize(appUserID: string) {
    const config = getRevenueCatConfig();
    if (!config || !appUserID.trim()) {
      return false;
    }

    if (!this.configured) {
      Purchases.setLogLevel(LOG_LEVEL.WARN);
      Purchases.configure({ apiKey: config.apiKey, appUserID });
      this.configured = true;
      return true;
    }

    await Purchases.logIn(appUserID);
    return true;
  }

  async logout() {
    if (!this.configured) {
      return;
    }
    try {
      await Purchases.logOut();
    } catch (error) {
      console.warn("[BillingService] Failed to log out RevenueCat:", error);
    }
  }

  async getOfferings(): Promise<PurchasesOffering | null> {
    if (!this.configured) {
      return null;
    }
    const offerings = await Purchases.getOfferings();
    const config = getRevenueCatConfig();
    if (!config) {
      return offerings.current ?? null;
    }
    return offerings.all[config.offeringId] ?? offerings.current ?? null;
  }

  async purchasePackage(pkg: PurchasesOffering["availablePackages"][number]) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return Purchases.purchasePackage(pkg);
  }

  async restorePurchases(): Promise<CustomerInfo> {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return Purchases.restorePurchases();
  }

  async presentCustomerCenter(activeProductId?: string | null) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    try {
      await RevenueCatUI.presentCustomerCenter();
      return;
    } catch (error) {
      if (Platform.OS !== "android") {
        throw error;
      }
    }

    const customerInfo = await this.getCustomerInfo();
    const fallbackProductId =
      activeProductId?.trim() ||
      customerInfo?.activeSubscriptions?.[0] ||
      customerInfo?.managementURL?.match(/sku=([^&]+)/)?.[1] ||
      null;
    await this.openGooglePlaySubscriptions(fallbackProductId);
  }

  async getCustomerInfo(): Promise<CustomerInfo | null> {
    if (!this.configured) {
      return null;
    }
    return Purchases.getCustomerInfo();
  }

  findProductForConfiguredSku(
    offering: PurchasesOffering | null,
    sku: string
  ): PurchasesOffering["availablePackages"][number] | null {
    if (!offering) {
      return null;
    }
    return (
      offering.availablePackages.find((pkg) => pkg.product.identifier === sku) ??
      null
    );
  }

  private async openGooglePlaySubscriptions(activeProductId?: string | null) {
    const config = getRevenueCatConfig();
    const sku = activeProductId?.trim();
    const packageName = config?.androidPackageName ?? "com.allcallall.mobile";
    const candidates = sku
      ? [
          `https://play.google.com/store/account/subscriptions?sku=${encodeURIComponent(sku)}&package=${encodeURIComponent(packageName)}`,
          `market://details?id=${encodeURIComponent(packageName)}`
        ]
      : [
          "https://play.google.com/store/account/subscriptions",
          `market://details?id=${encodeURIComponent(packageName)}`
        ];

    for (const url of candidates) {
      const supported = await Linking.canOpenURL(url);
      if (supported) {
        await Linking.openURL(url);
        return;
      }
    }

    throw new Error("unable to open Google Play subscription management");
  }
}

export default new BillingService();
