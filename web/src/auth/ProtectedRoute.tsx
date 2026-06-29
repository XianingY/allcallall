import { Navigate, Outlet, useLocation } from "react-router-dom";

import { useAuth } from "@/auth/AuthContext";

export function ProtectedRoute() {
  const { status } = useAuth();
  const location = useLocation();
  if (status === "loading") return <div className="app-loading"><span className="loading-indicator" />正在恢复会话</div>;
  if (status === "anonymous") return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  return <Outlet />;
}

export function AnonymousRoute() {
  const { status } = useAuth();
  if (status === "loading") return <div className="app-loading"><span className="loading-indicator" />正在恢复会话</div>;
  if (status === "authenticated") return <Navigate to="/inbox" replace />;
  return <Outlet />;
}

