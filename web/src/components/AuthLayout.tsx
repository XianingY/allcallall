import { Link } from "react-router-dom";
import type { PropsWithChildren } from "react";

export interface AuthActivityItem {
  actor: string;
  action: string;
  detail: string;
  tone: "person" | "agent" | "teammate" | "archive";
}

const AUTH_ACTIVITY: AuthActivityItem[] = [
  {
    actor: "客户经理",
    action: "提问",
    detail: "这家客户上一次会议承诺的退款时间是什么？",
    tone: "person",
  },
  {
    actor: "Agent",
    action: "总结",
    detail: "引用 3 段会议转写和 2 个工单，整理成可发送的答复。",
    tone: "agent",
  },
  {
    actor: "队友",
    action: "审批",
    detail: "确认答复内容，并补充后续跟进时间。",
    tone: "teammate",
  },
  {
    actor: "工作台",
    action: "归档",
    detail: "答复、来源和审批记录一起写入会议纪要。",
    tone: "archive",
  },
];

export function AuthActivityPanel({
  title,
  activity,
  children,
}: PropsWithChildren<{ title: string; activity: AuthActivityItem[] }>) {
  return (
    <section className="auth-activity">
      <h2>{title}</h2>
      <ol className="auth-activity-list" aria-label={title}>
        {activity.map((item) => (
          <li className="auth-activity-item" key={`${item.actor}-${item.action}`}>
            <span className={`auth-activity-dot auth-activity-${item.tone}`} aria-hidden="true" />
            <div>
              <strong>
                {item.actor} · {item.action}
              </strong>
              <p>{item.detail}</p>
            </div>
          </li>
        ))}
      </ol>
      {children}
    </section>
  );
}

export function AuthLayout({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return <main className="auth-page">
    <section className="auth-brand" aria-label="AllCallAll 产品概览">
      <div className="auth-brand-copy">
        <Link to="/" className="auth-brand-link">AllCallAll</Link>
        <p>会议协作与 Agent 工作台</p>
      </div>
      <AuthActivityPanel title="一次会话如何形成记录" activity={AUTH_ACTIVITY} />
    </section>
    <section className="auth-form-wrap"><div className="auth-form-card">
      <header><h1>{title}</h1><p>{description}</p></header>
      {children}
    </div></section>
  </main>;
}

export function FormError({ error }: { error: unknown }) {
  if (!error) return null;
  return <div className="status-error" role="alert">{error instanceof Error ? error.message : "请求失败，请稍后重试"}</div>;
}

export function FieldError({ message }: { message?: string }) {
  return message ? <span className="field-error">{message}</span> : null;
}
