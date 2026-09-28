import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, Clock3 } from "lucide-react";

import { listFollowUps, updateFollowUp } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { FOLLOWUP_STATUS, isFollowUpCompleted, isFollowUpPending } from "@allcallall/shared";

import { PageEmpty, PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationContext";

export function FollowUpsPage() {
  const { activeOrganization } = useOrganization(); const queryClient = useQueryClient(); const key = ["organizations", activeOrganization?.id, "follow-ups"];
  const query = useQuery({ queryKey: key, queryFn: listFollowUps, enabled: Boolean(activeOrganization) });
  const update = useMutation({ mutationFn: ({ id, status }: { id: number; status: string }) => updateFollowUp(id, { status }), onSuccess: () => queryClient.invalidateQueries({ queryKey: key }) });
  // These status values must match the backend's allowed set. This page used
  // to send "completed"/"pending", which the backend rejects, so every press
  // failed and the completed column stayed empty. See packages/shared.
  const pending = query.data?.filter((item) => isFollowUpPending(item.task.status)) ?? [];
  const completed = query.data?.filter((item) => isFollowUpCompleted(item.task.status)) ?? [];
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Next actions</p><h1>跟进</h1><p>集中处理通话和 Agent 生成的后续任务。</p></div></header><FormError error={update.error} />{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : (query.data?.length ?? 0) === 0 ? <PageEmpty label="还没有跟进任务" hint="通话和 Agent 生成的后续任务会出现在这里" /> : <div className="task-columns"><TaskGroup title="待处理" items={pending} update={(id) => update.mutate({ id, status: FOLLOWUP_STATUS.done })} disabled={update.isPending} /><TaskGroup title="已完成" items={completed} update={(id) => update.mutate({ id, status: FOLLOWUP_STATUS.open })} completed disabled={update.isPending} /></div>}</div>;
}

function TaskGroup({ title, items, update, completed = false, disabled = false }: { title: string; items: Awaited<ReturnType<typeof listFollowUps>>; update(id: number): void; completed?: boolean; disabled?: boolean }) { return <section><h2 className="column-title">{title}<span>{items.length}</span></h2><div className="list-stack">{items.map((item) => <article className="panel task-card" key={item.task.id}><button className="task-check" title={completed ? "重新打开" : "标记完成"} disabled={disabled} onClick={() => update(item.task.id)}>{completed ? <CheckCircle2 size={20} /> : <span />}</button><div><h3>{item.task.title}</h3><p>{item.task.description || item.peer?.display_name || "无描述"}</p><footer className={item.is_overdue ? "text-danger" : ""}><Clock3 size={14} />{item.task.due_at ? new Date(item.task.due_at).toLocaleString() : "无截止时间"}</footer></div></article>)}</div></section>; }

