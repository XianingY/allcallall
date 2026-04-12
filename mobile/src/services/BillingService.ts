import Purchases, {
  type CustomerInfo,
  type PurchasesOffering,
  LOG_LEVEL
} from "react-native-purchases";
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

  async presentCustomerCenter() {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return RevenueCatUI.presentCustomerCenter();
  }

  async getCustomerInfo(): Promise<CustomerInfo | null> {
    if (!this.configured) {
      return null;
    }
    return Purchases.getCustomerInfo();
  }
}

export default new BillingService();
