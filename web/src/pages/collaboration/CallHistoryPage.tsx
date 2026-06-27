import { useQuery } from "@tanstack/react-query";
import { PhoneCall } from "lucide-react";

import { listCallHistory } from "@/api/collaboration";
import { PageError, PageLoading } from "@/components/PageState";

export function CallHistoryPage() {
  const query = useQuery({ queryKey: ["calls", "history", 30], queryFn: () => listCallHistory(30) });
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Direct calls</p><h1>通话历史</h1><p>最近 30 天的一对一通话与跟进状态。</p></div></header>{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : <div className="panel table-wrap"><table><thead><tr><th>参与者</th><th>开始时间</th><th>状态</th><th>结束原因</th><th>跟进</th></tr></thead><tbody>{query.data?.map((call) => <tr key={call.id}><td><span className="table-primary"><PhoneCall size={15} />{call.caller_display_name} / {call.callee_display_name}</span></td><td>{new Date(call.started_at).toLocaleString()}</td><td>{call.status}</td><td>{call.end_reason || "-"}</td><td>{call.followup_status || "-"}</td></tr>)}</tbody></table></div>}</div>;
}
