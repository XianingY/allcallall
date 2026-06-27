import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Save } from "lucide-react";
import { useState } from "react";
import { Link, useParams } from "react-router-dom";

import { getDeal, listDealActivities, listPipelines, updateDeal, type Deal, type DealActivity, type PipelineStage } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function DealDetailPage() {
  const id = Number(useParams().dealId); const { activeOrganization } = useOrganization(); const queryClient = useQueryClient(); const orgId = activeOrganization?.id;
  const deal = useQuery({ queryKey: ["organizations", orgId, "deals", id], queryFn: () => getDeal(id), enabled: Boolean(orgId && id) });
  const activities = useQuery({ queryKey: ["organizations", orgId, "deals", id, "activities"], queryFn: () => listDealActivities(id), enabled: Boolean(orgId && id) });
  const pipelines = useQuery({ queryKey: ["organizations", orgId, "pipelines"], queryFn: listPipelines, enabled: Boolean(orgId) });
  const stages = pipelines.data?.flatMap((item) => item.stages) ?? [];
  if (deal.isLoading) return <PageLoading />; if (deal.isError || !deal.data) return <PageError error={deal.error ?? new Error("商机不存在")} />;
  return <DealEditor key={`${deal.data.id}-${deal.data.updated_at}`} deal={deal.data} stages={stages} activities={activities.data ?? []} activitiesLoading={activities.isLoading} onSaved={() => { void deal.refetch(); void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "deals"] }); }} />;
}

function DealEditor({ deal, stages, activities, activitiesLoading, onSaved }: { deal: Deal; stages: PipelineStage[]; activities: DealActivity[]; activitiesLoading: boolean; onSaved(): void }) {
  const [form, setForm] = useState({ title: deal.title, description: deal.description ?? "", value: String(deal.value_cents / 100), stage_id: deal.stage_id ?? 0, status: deal.status });
  const save = useMutation({ mutationFn: () => updateDeal(deal.id, { title: form.title, description: form.description, value_cents: Math.round(Number(form.value) * 100), stage_id: form.stage_id, status: form.status }), onSuccess: onSaved });
  return <div className="page"><header className="page-header"><div><Link className="back-link" to="/deals"><ArrowLeft size={16} />返回 Pipeline</Link><h1>{deal.title}</h1></div><button className="button-primary" onClick={() => save.mutate()}><Save size={17} />保存</button></header><div className="detail-grid"><section className="panel panel-body"><h2>商机信息</h2><FormError error={save.error} />{save.isSuccess && <div className="status-success">已保存</div>}<div className="form-stack mt-4"><label>标题<input className="field" value={form.title} onChange={(event) => setForm({ ...form, title: event.target.value })} /></label><label>描述<textarea className="field" rows={5} value={form.description} onChange={(event) => setForm({ ...form, description: event.target.value })} /></label><label>金额<input className="field" type="number" value={form.value} onChange={(event) => setForm({ ...form, value: event.target.value })} /></label><label>阶段<select className="field" value={form.stage_id} onChange={(event) => setForm({ ...form, stage_id: Number(event.target.value) })}>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.name}</option>)}</select></label><label>状态<select className="field" value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value })}><option value="open">进行中</option><option value="won">赢单</option><option value="lost">输单</option></select></label></div></section><section className="panel panel-body"><h2>活动轨迹</h2>{activitiesLoading ? <PageLoading /> : <div className="timeline">{activities.map((item) => <article key={item.id}><span /><div><strong>{item.summary}</strong><p>{item.type} · {new Date(item.created_at).toLocaleString()}</p></div></article>)}</div>}</section></div></div>;
}
