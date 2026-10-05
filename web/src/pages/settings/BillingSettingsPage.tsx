import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { formatDateTime } from "@allcallall/shared";
import { CreditCard } from "lucide-react";
import { useState } from "react";

import { getEntitlements, getUsage } from "@/api/platform";
import { useAuth } from "@/auth/AuthContext";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import {
  isBillingConfigured,
  openRevenueCatCheckout,
  openRevenueCatPortal,
  preloadRevenueCat,
} from "@/platform/billing";

const dateTime = (value?: string | null) => value ? formatDateTime(value) : "-";

export function BillingSettingsPage() {
  const { user } = useAuth();
  const queryClient = useQueryClient();
  const entitlements = useQuery({ queryKey: ["billing", "entitlements"], queryFn: getEntitlements });
  const usage = useQuery({ queryKey: ["billing", "usage"], queryFn: getUsage });
  // Entitlements are written by an async webhook, so re-fetching right after
  // checkout returns the old "Free" tier. Showing that with no explanation
  // makes a successful payment look failed - and invites paying twice.
  const [syncing, setSyncing] = useState(false);
  const preloadBillingSdk = () => {
    void preloadRevenueCat().catch(() => {
      // Speculative preloads are best-effort; failures are surfaced only
      // if the user actually invokes checkout/portal.
    });
  };
  const purchase = useMutation({
    mutationFn: async () => { if (!user) throw new Error("not signed in"); return openRevenueCatCheckout(user); },
    onSuccess: async () => {
      setSyncing(true);
      try {
        for (let attempt = 0; attempt < 6; attempt += 1) {
          const latest = await queryClient.fetchQuery({ queryKey: ["billing", "entitlements"], queryFn: getEntitlements });
          if (latest?.tier === "premium") {
            break;
          }
          await new Promise((resolve) => setTimeout(resolve, 1500));
        }
        void queryClient.invalidateQueries({ queryKey: ["billing"] });
      } finally {
        setSyncing(false);
      }
    },
  });
  const portal = useMutation({ mutationFn: async () => { if (!user) throw new Error("not signed in"); return openRevenueCatPortal(user); } });
  return <SettingsPanel title="订阅与用量" description="后端 entitlement 是最终权限来源，RevenueCat 只负责 Web Billing 入口。"><FormError error={purchase.error || portal.error} />{entitlements.isLoading ? <PageLoading /> : entitlements.isError ? <PageError error={entitlements.error} /> : <div className="billing-summary"><div><span>当前版本</span><strong>{entitlements.data?.tier === "premium" ? "Premium" : "Free"}</strong></div><div className="button-row"><button className="button-primary" disabled={!isBillingConfigured() || purchase.isPending} onClick={() => purchase.mutate()} onPointerEnter={preloadBillingSdk} onFocus={preloadBillingSdk}><CreditCard size={17} />升级</button><button className="button-secondary" disabled={!isBillingConfigured() || portal.isPending} onClick={() => portal.mutate()} onPointerEnter={preloadBillingSdk} onFocus={preloadBillingSdk}>管理订阅</button></div>{!isBillingConfigured() && <p className="text-muted">未配置 RevenueCat public API key，生产环境通过 runtime config 注入。</p>}
    {syncing && <p className="text-muted" role="status">购买已提交，正在等待服务端同步权益…</p>}</div>}<section className="settings-section"><h3>权益</h3>{entitlements.data?.entitlements.length ? entitlements.data.entitlements.map((item) => <div className="data-row" key={item.id}><div><strong>{item.entitlement}</strong><small>{item.tier} · {item.status} · {item.source}</small>{item.expires_at && <small>到期 {dateTime(item.expires_at)}</small>}</div></div>) : <div className="inline-empty">暂无付费权益</div>}</section><section className="settings-section"><h3>用量</h3>{usage.isLoading ? <PageLoading /> : usage.isError ? <PageError error={usage.error} /> : usage.data?.map((item) => <div className="data-row" key={`${item.feature}-${item.period_key}`}><div><strong>{item.feature}</strong><small>{item.period_key} · {item.used_units}/{item.unlimited ? "无限" : item.limit_units} {item.unit}</small></div><span>{item.unlimited ? "unlimited" : `剩余 ${item.remaining_units}`}</span></div>)}</section></SettingsPanel>;
}

function SettingsPanel({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <div className="panel panel-body settings-panel"><header><h2>{title}</h2><p>{description}</p></header>{children}</div>;
}
