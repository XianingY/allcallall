import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { sendVerificationCode, verifyEmailCode } from "@/api/identity";
import { AuthLayout, FormError } from "@/components/AuthLayout";

export function VerifyEmailPage() {
  const [params] = useSearchParams();
  const [email, setEmail] = useState(params.get("email") ?? "");
  const [code, setCode] = useState("");
  const [error, setError] = useState<unknown>();
  const [done, setDone] = useState(false);
  const verify = async () => {
    setError(undefined);
    try { await verifyEmailCode(email, code, "register"); setDone(true); } catch (caught) { setError(caught); }
  };
  return <AuthLayout title="验证邮箱" description="验证结果仅在短时间内有效">
    <div className="form-stack">
      <FormError error={error} />
      {done ? <div className="status-success">邮箱已验证。请返回注册页完成账号创建。</div> : <>
        <label>邮箱<input className="field" type="email" value={email} onChange={(event) => setEmail(event.target.value)} /></label>
        <label>验证码<input className="field" inputMode="numeric" maxLength={6} value={code} onChange={(event) => setCode(event.target.value)} /></label>
        <div className="button-row"><button className="button-secondary" onClick={() => void sendVerificationCode(email, "register").catch(setError)}>发送验证码</button><button className="button-primary" onClick={() => void verify()}>完成验证</button></div>
      </>}
      <Link to="/register">返回注册</Link>
    </div>
  </AuthLayout>;
}

