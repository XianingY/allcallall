import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Clock3 } from "lucide-react";

import { listFollowUps, updateFollowUp } from "@/api/collaboration";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function FollowUpsPage() {
  const { activeOrganization } = useOrganization(); const queryClient = useQueryClient(); const key = ["organizations", activeOrganization?.id, "follow-ups"];
  const query = useQuery({ queryKey: key, queryFn: listFollowUps, enabled: Boolean(activeOrganization) });
  const update = useMutation({ mutationFn: ({ id, status }: { id: number; status: string }) => updateFollowUp(id, { status }), onSuccess: () => queryClient.invalidateQueries({ queryKey: key }) });
  const pending = query.data?.filter((item) => item.task.status !== "completed") ?? []; const completed = query.data?.filter((item) => item.task.status === "completed") ?? [];
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Next actions</p><h1>跟进</h1><p>集中处理通话和 Agent 生成的后续任务。</p></div></header>{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : <div className="task-columns"><TaskGroup title="待处理" items={pending} update={(id) => update.mutate({ id, status: "completed" })} /><TaskGroup title="已完成" items={completed} update={(id) => update.mutate({ id, status: "pending" })} completed /></div>}</div>;
}

function TaskGroup({ title, items, update, completed = false }: { title: string; items: Awaited<ReturnType<typeof listFollowUps>>; update(id: number): void; completed?: boolean }) { return <section><h2 className="column-title">{title}<span>{items.length}</span></h2><div className="list-stack">{items.map((item) => <article className="panel task-card" key={item.task.id}><button className="task-check" title={completed ? "重新打开" : "标记完成"} onClick={() => update(item.task.id)}>{completed ? <CheckCircle2 size={20} /> : <span />}</button><div><h3>{item.task.title}</h3><p>{item.task.description || item.peer?.display_name || "无描述"}</p><footer className={item.is_overdue ? "text-danger" : ""}><Clock3 size={14} />{item.task.due_at ? new Date(item.task.due_at).toLocaleString() : "无截止时间"}</footer></div></article>)}</div></section>; }

