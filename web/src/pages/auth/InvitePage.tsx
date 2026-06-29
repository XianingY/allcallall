import { Building2 } from "lucide-react";
import { useState } from "react";
import { Navigate, useNavigate, useParams } from "react-router-dom";

import { acceptOrganizationInvite } from "@/api/identity";
import { useAuth } from "@/auth/AuthContext";
import { AuthLayout, FormError } from "@/components/AuthLayout";

export function InvitePage() {
  const { code = "" } = useParams();
  const { status } = useAuth();
  const navigate = useNavigate();
  const [error, setError] = useState<unknown>();
  if (!code) return <Navigate to="/login" replace />;
  if (status === "anonymous") return <Navigate to="/login" replace state={{ from: `/invite/${code}` }} />;
  const accept = async () => { setError(undefined); try { await acceptOrganizationInvite(code); navigate("/organizations", { replace: true }); } catch (caught) { setError(caught); } };
  return <AuthLayout title="接受组织邀请" description="确认后该组织会出现在你的工作空间列表中"><div className="form-stack"><FormError error={error} /><div className="invite-code"><Building2 size={22} /><span>邀请码</span><strong>{code}</strong></div><button className="button-primary" onClick={() => void accept()}>接受邀请</button><button className="button-secondary" onClick={() => navigate("/inbox")}>暂不加入</button></div></AuthLayout>;
}

