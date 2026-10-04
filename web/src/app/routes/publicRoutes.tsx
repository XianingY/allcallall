import { Route } from "react-router-dom";

import { AnonymousRoute } from "@/auth/ProtectedRoute";
import { ForgotPasswordPage } from "@/pages/auth/ForgotPasswordPage";
import { InvitePage } from "@/pages/auth/InvitePage";
import { LoginPage } from "@/pages/auth/LoginPage";
import { RegisterPage } from "@/pages/auth/RegisterPage";
import { VerifyEmailPage } from "@/pages/auth/VerifyEmailPage";

export function publicRoutes() {
  return (
    <>
      <Route element={<AnonymousRoute />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
        <Route path="/verify-email" element={<VerifyEmailPage />} />
        <Route path="/forgot-password" element={<ForgotPasswordPage />} />
      </Route>
      <Route path="/invite/:code" element={<InvitePage />} />
    </>
  );
}
