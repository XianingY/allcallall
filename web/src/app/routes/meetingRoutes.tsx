import { Route } from "react-router-dom";

import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { LazyLoad } from "@/components/LazyLoad";
import { PageLoading } from "@/components/PageState";

export function meetingRoutes() {
  return (
    <Route element={<ProtectedRoute />}>
      <Route
        path="/meetings/:roomId/preflight"
        element={<LazyLoad loader={() => import("@/pages/meetings/MeetingPreflightPage").then((module) => ({ default: module.MeetingPreflightPage }))} fallback={<PageLoading />} />}
      />
      <Route
        path="/meetings/:roomId"
        element={<LazyLoad loader={() => import("@/pages/meetings/MeetingRoomPage").then((module) => ({ default: module.MeetingRoomPage }))} fallback={<PageLoading />} />}
      />
    </Route>
  );
}
