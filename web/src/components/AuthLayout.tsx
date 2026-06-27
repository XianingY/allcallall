import { Link } from "react-router-dom";

export function AuthLayout({ title, description, children }: { title: string; description: string; children: React.ReactNode }) {
  return <main className="auth-page">
    <section className="auth-brand" aria-label="AllCallAll">
      <Link to="/" className="text-xl font-bold">AllCallAll</Link>
      <p>会议协作与 Agent 工作台</p>
      <div className="auth-value"><span>01</span><p>会议、会话和业务上下文统一沉淀</p></div>
      <div className="auth-value"><span>02</span><p>转写、检索、复盘与审批形成可追踪闭环</p></div>
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

