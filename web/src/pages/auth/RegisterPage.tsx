import { zodResolver } from "@hookform/resolvers/zod";
import { Check, Mail } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate } from "react-router-dom";
import { z } from "zod";

import { getLegal, sendVerificationCode, verifyEmailCode } from "@/api/identity";
import { useAuth } from "@/auth/AuthProvider";
import { AuthLayout, FieldError, FormError } from "@/components/AuthLayout";
import { useQuery } from "@tanstack/react-query";

const schema = z.object({
  displayName: z.string().trim().min(2, "显示名称至少 2 个字符"),
  email: z.string().email("请输入有效邮箱"),
  code: z.string().regex(/^\d{6}$/, "请输入 6 位验证码"),
  password: z.string().min(8, "密码至少 8 个字符"),
  confirmPassword: z.string(),
  legal: z.literal(true, { error: "请阅读并同意条款" }),
}).refine((value) => value.password === value.confirmPassword, { path: ["confirmPassword"], message: "两次密码不一致" });
type Values = z.infer<typeof schema>;

export function RegisterPage() {
  const { register: createAccount } = useAuth();
  const navigate = useNavigate();
  const legal = useQuery({ queryKey: ["legal"], queryFn: getLegal });
  const [sentTo, setSentTo] = useState("");
  const [verifiedEmail, setVerifiedEmail] = useState("");
  const [error, setError] = useState<unknown>();
  const { register, handleSubmit, getValues, formState } = useForm<Values>({ resolver: zodResolver(schema) });

  const send = async () => {
    const email = getValues("email");
    if (!z.string().email().safeParse(email).success) { setError(new Error("请先输入有效邮箱")); return; }
    setError(undefined);
    try { await sendVerificationCode(email, "register"); setSentTo(email); setVerifiedEmail(""); } catch (caught) { setError(caught); }
  };

  const submit = handleSubmit(async (values) => {
    setError(undefined);
    try {
      if (verifiedEmail !== values.email) { await verifyEmailCode(values.email, values.code, "register"); setVerifiedEmail(values.email); }
      await createAccount({ email: values.email, display_name: values.displayName, password: values.password, accept_current_legal: true });
      navigate("/organizations", { replace: true });
    } catch (caught) { setError(caught); }
  });

  return <AuthLayout title="创建账号" description="验证码通过后即可进入个人组织">
    <form className="form-stack" onSubmit={submit}>
      <FormError error={error} />
      <label>显示名称<input className="field" autoComplete="name" {...register("displayName")} /><FieldError message={formState.errors.displayName?.message} /></label>
      <label>邮箱<input className="field" type="email" autoComplete="email" {...register("email")} /><FieldError message={formState.errors.email?.message} /></label>
      <div className="verification-row">
        <label>验证码<input className="field" inputMode="numeric" maxLength={6} autoComplete="one-time-code" {...register("code")} /><FieldError message={formState.errors.code?.message} /></label>
        <button className="button-secondary" type="button" onClick={send}><Mail size={16} />{sentTo === getValues("email") ? "重新发送" : "发送验证码"}</button>
      </div>
      {sentTo && <p className="field-hint">验证码已发送至 {sentTo}</p>}
      <label>密码<input className="field" type="password" autoComplete="new-password" {...register("password")} /><FieldError message={formState.errors.password?.message} /></label>
      <label>确认密码<input className="field" type="password" autoComplete="new-password" {...register("confirmPassword")} /><FieldError message={formState.errors.confirmPassword?.message} /></label>
      <label className="check-field"><input type="checkbox" {...register("legal")} /><span>我已阅读并同意 <a href={legal.data?.terms_url ?? "/legal/terms"} target="_blank" rel="noreferrer">服务条款</a> 与 <a href={legal.data?.privacy_policy_url ?? "/legal/privacy"} target="_blank" rel="noreferrer">隐私政策</a></span></label>
      <FieldError message={formState.errors.legal?.message} />
      <button className="button-primary w-full" disabled={formState.isSubmitting}><Check size={17} />验证并注册</button>
    </form>
    <p className="auth-footer">已有账号？ <Link to="/login">登录</Link></p>
  </AuthLayout>;
}

