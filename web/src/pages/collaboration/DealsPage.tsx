import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CircleDollarSign, Plus, X } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { createDeal, listDeals, listPipelines, updateDeal } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

const money = (cents: number, currency: string) => new Intl.NumberFormat(undefined, { style: "currency", currency: currency || "CNY", maximumFractionDigits: 0 }).format(cents / 100);

export function DealsPage() {
  const { activeOrganization } = useOrganization(); const queryClient = useQueryClient(); const orgId = activeOrganization?.id; const [open, setOpen] = useState(false);
  const pipelines = useQuery({ queryKey: ["organizations", orgId, "pipelines"], queryFn: listPipelines, enabled: Boolean(orgId) });
  const deals = useQuery({ queryKey: ["organizations", orgId, "deals"], queryFn: listDeals, enabled: Boolean(orgId) });
  const move = useMutation({ mutationFn: ({ id, stage_id }: { id: number; stage_id: number }) => updateDeal(id, { stage_id }), onSuccess: () => queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "deals"] }) });
  const stages = pipelines.data?.find((item) => item.is_default)?.stages ?? pipelines.data?.[0]?.stages ?? [];
  return <div className="page page-wide"><header className="page-header"><div><p className="eyebrow">Pipeline</p><h1>商机</h1><p>按阶段推进商机，并保留会话、联系人和活动轨迹。</p></div><button className="button-primary" onClick={() => setOpen(true)}><Plus size={17} />新建商机</button></header>
    {pipelines.isLoading || deals.isLoading ? <PageLoading /> : pipelines.isError || deals.isError ? <PageError error={pipelines.error || deals.error} /> : <div className="deal-board">{stages.map((stage) => { const items = deals.data?.filter((deal) => deal.stage_id === stage.id) ?? []; return <section className="deal-column" key={stage.id}><header><h2>{stage.name}</h2><span>{items.length}</span></header><div>{items.map((deal) => <article className="panel deal-card" key={deal.id}><Link to={`/deals/${deal.id}`}><h3>{deal.title}</h3><p>{deal.description || "暂无描述"}</p><strong>{money(deal.value_cents, deal.currency)}</strong></Link><select aria-label={`移动 ${deal.title}`} value={deal.stage_id ?? ""} onChange={(event) => move.mutate({ id: deal.id, stage_id: Number(event.target.value) })}>{stages.map((option) => <option key={option.id} value={option.id}>{option.name}</option>)}</select></article>)}</div></section>; })}</div>}
    <NewDealDialog open={open} onOpenChange={setOpen} stages={stages} orgId={orgId} />
  </div>;
}

function NewDealDialog({ open, onOpenChange, stages, orgId }: { open: boolean; onOpenChange(open: boolean): void; stages: Awaited<ReturnType<typeof listPipelines>>[number]["stages"]; orgId?: number }) {
  const queryClient = useQueryClient(); const [title, setTitle] = useState(""); const [description, setDescription] = useState(""); const [value, setValue] = useState(""); const [stageId, setStageId] = useState<number | undefined>();
  const create = useMutation({ mutationFn: () => createDeal({ title, description, value_cents: Math.round(Number(value || 0) * 100), currency: "CNY", stage_id: stageId ?? stages[0]?.id }), onSuccess: () => { onOpenChange(false); setTitle(""); setDescription(""); setValue(""); void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "deals"] }); } });
  return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>新建商机</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>商机默认归属当前组织。</Dialog.Description><FormError error={create.error} /><div className="form-stack"><label>标题<input className="field" value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>描述<textarea className="field" value={description} onChange={(event) => setDescription(event.target.value)} /></label><label>金额（CNY）<div className="input-icon"><CircleDollarSign size={16} /><input type="number" min="0" value={value} onChange={(event) => setValue(event.target.value)} /></div></label><label>阶段<select className="field" value={stageId ?? stages[0]?.id ?? ""} onChange={(event) => setStageId(Number(event.target.value))}>{stages.map((stage) => <option key={stage.id} value={stage.id}>{stage.name}</option>)}</select></label><button className="button-primary" disabled={!title.trim()} onClick={() => create.mutate()}>创建商机</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>;
}

