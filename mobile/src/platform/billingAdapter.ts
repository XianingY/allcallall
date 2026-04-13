import { Linking, Platform } from "react-native";

import { getRevenueCatConfig } from "../api/commercial";

export interface BillingCustomerInfo {
  activeSubscriptions: string[];
  managementURL?: string | null;
}

export interface BillingPackage {
  identifier: string;
  product: {
    identifier: string;
    title: string;
    description?: string;
    priceString?: string;
  };
}

export interface BillingOffering {
  identifier?: string;
  availablePackages: BillingPackage[];
}

export interface BillingAdapter {
  initialize(appUserID: string): Promise<boolean>;
  logout(): Promise<void>;
  getOfferings(): Promise<BillingOffering | null>;
  purchasePackage(pkg: BillingPackage): Promise<void>;
  restorePurchases(): Promise<BillingCustomerInfo>;
  presentCustomerCenter(activeProductId?: string | null): Promise<void>;
  getCustomerInfo(): Promise<BillingCustomerInfo | null>;
  isSupported(): boolean;
}

const webAdapter: BillingAdapter = {
  async initialize() {
    return false;
  },
  async logout() {
    return;
  },
  async getOfferings() {
    return null;
  },
  async purchasePackage() {
    throw new Error("billing is not supported on web yet");
  },
  async restorePurchases() {
    throw new Error("billing is not supported on web yet");
  },
  async presentCustomerCenter() {
    throw new Error("billing is not supported on web yet");
  },
  async getCustomerInfo() {
    return null;
  },
  isSupported() {
    return false;
  },
};

class NativeBillingAdapter implements BillingAdapter {
  private configured = false;

  isSupported() {
    return true;
  }

  async initialize(appUserID: string) {
    const config = getRevenueCatConfig();
    if (!config || !appUserID.trim()) {
      return false;
    }

    const Purchases = require("react-native-purchases").default;
    const { LOG_LEVEL } = require("react-native-purchases");
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
    const Purchases = require("react-native-purchases").default;
    await Purchases.logOut();
  }

  async getOfferings(): Promise<BillingOffering | null> {
    if (!this.configured) {
      return null;
    }
    const Purchases = require("react-native-purchases").default;
    const offerings = await Purchases.getOfferings();
    const config = getRevenueCatConfig();
    return offerings.all[config?.offeringId ?? ""] ?? offerings.current ?? null;
  }

  async purchasePackage(pkg: BillingPackage) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    const Purchases = require("react-native-purchases").default;
    await Purchases.purchasePackage(pkg);
  }

  async restorePurchases(): Promise<BillingCustomerInfo> {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    const Purchases = require("react-native-purchases").default;
    return Purchases.restorePurchases();
  }

  async presentCustomerCenter(activeProductId?: string | null) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    const RevenueCatUI = require("react-native-purchases-ui").default;
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

  async getCustomerInfo(): Promise<BillingCustomerInfo | null> {
    if (!this.configured) {
      return null;
    }
    const Purchases = require("react-native-purchases").default;
    return Purchases.getCustomerInfo();
  }

  private async openGooglePlaySubscriptions(activeProductId?: string | null) {
    const config = getRevenueCatConfig();
    const sku = activeProductId?.trim();
    const packageName = config?.androidPackageName ?? "com.allcallall.mobile";
    const candidates = sku
      ? [
          `https://play.google.com/store/account/subscriptions?sku=${encodeURIComponent(sku)}&package=${encodeURIComponent(packageName)}`,
          `market://details?id=${encodeURIComponent(packageName)}`,
        ]
      : [
          "https://play.google.com/store/account/subscriptions",
          `market://details?id=${encodeURIComponent(packageName)}`,
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

const billingAdapter: BillingAdapter =
  Platform.OS === "web" ? webAdapter : new NativeBillingAdapter();

export default billingAdapter;
