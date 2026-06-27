import { useEffect, useState } from "react";
import { X } from "lucide-react";

import { listenForegroundPush } from "@/platform/push";

interface ForegroundNotification {
  title: string;
  body: string;
  url?: string;
}

export function PushNotificationBridge() {
  const [notification, setNotification] = useState<ForegroundNotification | null>(null);

  useEffect(() => {
    let unsubscribe: (() => void) | undefined;
    void listenForegroundPush((payload) => setNotification(payload)).then((cleanup) => { unsubscribe = cleanup; });
    return () => unsubscribe?.();
  }, []);

  if (!notification) return null;
  return <div className="web-push-toast" role="status" aria-live="polite">
    <button className="icon-button" aria-label="关闭通知" onClick={() => setNotification(null)}><X size={16} /></button>
    <strong>{notification.title}</strong>
    {notification.body && <p>{notification.body}</p>}
    {notification.url && <a href={notification.url}>打开</a>}
  </div>;
}
