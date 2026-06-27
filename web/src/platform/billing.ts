import { runtimeConfig } from "@/lib/runtime-config";

let configuredForUser: string | null = null;
let purchasesClass: (typeof import("@revenuecat/purchases-js"))["Purchases"] | null = null;

export const isBillingConfigured = () => Boolean(runtimeConfig.revenueCatPublicApiKey);

async function loadPurchases() {
  if (!purchasesClass) purchasesClass = (await import("@revenuecat/purchases-js")).Purchases;
  return purchasesClass;
}

export async function getPurchasesForUser(userId: number) {
  const Purchases = await loadPurchases();
  const appUserId = `user:${userId}`;
  if (configuredForUser === appUserId && Purchases.isConfigured()) return Purchases.getSharedInstance();
  configuredForUser = appUserId;
  return Purchases.configure({
    apiKey: runtimeConfig.revenueCatPublicApiKey!,
    appUserId,
  });
}

export async function openRevenueCatCheckout(user: { id: number; email: string }) {
  if (!isBillingConfigured()) throw new Error("RevenueCat Web Billing 未配置。");
  const purchases = await getPurchasesForUser(user.id);
  const offerings = await purchases.getOfferings();
  const candidate = offerings.current?.availablePackages?.[0] ?? offerings.all[Object.keys(offerings.all)[0]]?.availablePackages?.[0];
  if (!candidate) throw new Error("RevenueCat 当前没有可购买的 Web offering。");
  await purchases.purchase({ rcPackage: candidate, customerEmail: user.email });
  return purchases.getCustomerInfo();
}

export async function openRevenueCatPortal(user: { id: number }) {
  if (!isBillingConfigured()) throw new Error("RevenueCat Web Billing 未配置。");
  const purchases = await getPurchasesForUser(user.id);
  const info = await purchases.getCustomerInfo();
  if (!info.managementURL) throw new Error("当前账号没有可打开的订阅管理页面。");
  window.open(info.managementURL, "_blank", "noopener,noreferrer");
}
