import billingAdapter, {
  type BillingCustomerInfo as CustomerInfo,
  type BillingOffering as PurchasesOffering,
} from "../platform/billingAdapter";

class BillingService {
  private configured = false;

  async initialize(appUserID: string) {
    const initialized = await billingAdapter.initialize(appUserID);
    this.configured = initialized || this.configured;
    return initialized;
  }

  async logout() {
    if (!this.configured) {
      return;
    }
    try {
      await billingAdapter.logout();
    } catch (error) {
      console.warn("[BillingService] Failed to log out RevenueCat:", error);
    }
  }

  async getOfferings(): Promise<PurchasesOffering | null> {
    if (!this.configured) {
      return null;
    }
    return billingAdapter.getOfferings();
  }

  async purchasePackage(pkg: PurchasesOffering["availablePackages"][number]) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return billingAdapter.purchasePackage(pkg);
  }

  async restorePurchases(): Promise<CustomerInfo> {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return billingAdapter.restorePurchases();
  }

  async presentCustomerCenter(activeProductId?: string | null) {
    if (!this.configured) {
      throw new Error("billing not configured");
    }
    return billingAdapter.presentCustomerCenter(activeProductId);
  }

  async getCustomerInfo(): Promise<CustomerInfo | null> {
    if (!this.configured) {
      return null;
    }
    return billingAdapter.getCustomerInfo();
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
}

export default new BillingService();
