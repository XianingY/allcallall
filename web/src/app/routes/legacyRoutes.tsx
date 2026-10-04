import { Navigate, Route, useParams } from "react-router-dom";

export function legacyRoutes() {
  return (
    <>
      <Route path="/rooms/:roomId" element={<LegacyRoomRedirect />} />
      <Route path="*" element={<Navigate to="/inbox" replace />} />
    </>
  );
}

/** Sends room links shared by older mobile builds to the canonical route. */
function LegacyRoomRedirect() {
  const { roomId } = useParams();
  return <Navigate to={`/meetings/${roomId ?? ""}`} replace />;
}
