import { zodResolver } from "@hookform/resolvers/zod";
import { ArrowRight } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { z } from "zod";

import { useAuth } from "@/auth/AuthContext";
import { AuthLayout, FieldError, FormError } from "@/components/AuthLayout";

const schema = z.object({ email: z.string().email("请输入有效邮箱"), password: z.string().min(1, "请输入密码") });
type Values = z.infer<typeof schema>;

export function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [error, setError] = useState<unknown>();
  const { register, handleSubmit, formState } = useForm<Values>({ resolver: zodResolver(schema) });
  const submit = handleSubmit(async (values) => {
    setError(undefined);
    try {
      await login(values.email, values.password);
      const from = (location.state as { from?: string } | null)?.from ?? "/inbox";
      navigate(from, { replace: true });
    } catch (caught) { setError(caught); }
  });
  return <AuthLayout title="登录工作台" description="使用 AllCallAll 账号继续">
    <form className="form-stack" onSubmit={submit}>
      <FormError error={error} />
      <label>邮箱<input className="field" type="email" autoComplete="email" {...register("email")} /><FieldError message={formState.errors.email?.message} /></label>
      <label>密码<input className="field" type="password" autoComplete="current-password" {...register("password")} /><FieldError message={formState.errors.password?.message} /></label>
      <div className="form-row"><Link to="/forgot-password">忘记密码</Link></div>
      <button className="button-primary w-full" disabled={formState.isSubmitting}>登录 <ArrowRight size={17} /></button>
    </form>
    <p className="auth-footer">还没有账号？ <Link to="/register">创建账号</Link></p>
  </AuthLayout>;
}

