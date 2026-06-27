import { useState } from "react";
import { Link } from "react-router-dom";

import { confirmPasswordReset, sendPasswordReset } from "@/api/identity";
import { AuthLayout, FormError } from "@/components/AuthLayout";

export function ForgotPasswordPage() {
  const [step, setStep] = useState<"email" | "reset" | "done">("email");
  const [email, setEmail] = useState("");
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState<unknown>();
  const send = async () => { setError(undefined); try { await sendPasswordReset(email); setStep("reset"); } catch (caught) { setError(caught); } };
  const reset = async () => { setError(undefined); try { await confirmPasswordReset({ email, code, new_password: password, confirm_password: confirm }); setStep("done"); } catch (caught) { setError(caught); } };
  return <AuthLayout title="重置密码" description="验证码将在有效期后自动失效">
    <div className="form-stack"><FormError error={error} />
      {step === "email" && <><label>账号邮箱<input className="field" type="email" value={email} onChange={(event) => setEmail(event.target.value)} /></label><button className="button-primary" onClick={() => void send()}>发送重置码</button></>}
      {step === "reset" && <><p className="field-hint">重置码已发送至 {email}</p><label>验证码<input className="field" inputMode="numeric" maxLength={6} value={code} onChange={(event) => setCode(event.target.value)} /></label><label>新密码<input className="field" type="password" value={password} onChange={(event) => setPassword(event.target.value)} /></label><label>确认新密码<input className="field" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></label><button className="button-primary" onClick={() => void reset()}>更新密码</button></>}
      {step === "done" && <div className="status-success">密码已更新，现在可以使用新密码登录。</div>}
      <Link to="/login">返回登录</Link>
    </div>
  </AuthLayout>;
}
