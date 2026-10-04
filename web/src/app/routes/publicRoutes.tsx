import { Suspense, lazy, type ComponentType } from "react";
import { Route } from "react-router-dom";

import { AnonymousRoute } from "@/auth/ProtectedRoute";
import { PageLoading } from "@/components/PageState";

const ForgotPasswordPage = lazy(() => import("@/pages/auth/ForgotPasswordPage").then((module) => ({ default: module.ForgotPasswordPage })));
const InvitePage = lazy(() => import("@/pages/auth/InvitePage").then((module) => ({ default: module.InvitePage })));
const LoginPage = lazy(() => import("@/pages/auth/LoginPage").then((module) => ({ default: module.LoginPage })));
const RegisterPage = lazy(() => import("@/pages/auth/RegisterPage").then((module) => ({ default: module.RegisterPage })));
const VerifyEmailPage = lazy(() => import("@/pages/auth/VerifyEmailPage").then((module) => ({ default: module.VerifyEmailPage })));

const loading = <PageLoading />;
const lazyPage = (Page: ComponentType) => (
  <Suspense fallback={loading}>
    <Page />
  </Suspense>
);

export function publicRoutes() {
  return (
    <>
      <Route element={<AnonymousRoute />}>
        <Route path="/login" element={lazyPage(LoginPage)} />
        <Route path="/register" element={lazyPage(RegisterPage)} />
        <Route path="/verify-email" element={lazyPage(VerifyEmailPage)} />
        <Route path="/forgot-password" element={lazyPage(ForgotPasswordPage)} />
      </Route>
      <Route path="/invite/:code" element={lazyPage(InvitePage)} />
    </>
  );
}
