import { Routes } from "react-router-dom";

import { ErrorBoundary } from "@/components/ErrorBoundary";
import { legacyRoutes } from "@/app/routes/legacyRoutes";
import { meetingRoutes } from "@/app/routes/meetingRoutes";
import { publicRoutes } from "@/app/routes/publicRoutes";
import { workspaceRoutes } from "@/app/routes/workspaceRoutes";

export function App() {
  return (
    <ErrorBoundary>
      <Routes>
        {publicRoutes()}
        {workspaceRoutes()}
        {meetingRoutes()}
        {legacyRoutes()}
      </Routes>
    </ErrorBoundary>
  );
}
