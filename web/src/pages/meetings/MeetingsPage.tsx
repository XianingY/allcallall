import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarPlus, Radio, Users, X } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";

import { createRoom, listRooms } from "@/api/meetings";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationContext";

export function MeetingsPage() {
  const { activeOrganization } = useOrganization(); const orgId = activeOrganization?.id; const [open, setOpen] = useState(false);
  const query = useQuery({ queryKey: ["organizations", orgId, "rooms"], queryFn: listRooms, enabled: Boolean(orgId), refetchInterval: 10_000 });
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Meetings</p><h1>会议</h1><p>最多 6 人，录制结束后自动进入转写链路。</p></div><button className="button-primary" onClick={() => setOpen(true)}><CalendarPlus size={17} />创建会议</button></header>{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : <div className="meeting-grid">{query.data?.map((item) => <article className="panel meeting-card" key={item.room.id}><header><span className={`meeting-state ${item.is_active ? "active" : ""}`}><Radio size={14} />{item.is_active ? "进行中" : item.room.status}</span><time>{new Date(item.room.created_at).toLocaleDateString()}</time></header><h2>{item.room.title}</h2><p>{item.conversation_title || "独立会议"}</p><footer><span><Users size={15} />{item.participant_count}/6</span><Link className="button-secondary" to={`/meetings/${item.room.id}/preflight`}>{item.is_active ? "加入" : "查看"}</Link></footer></article>)}</div>}<CreateMeetingDialog open={open} onOpenChange={setOpen} orgId={orgId} /></div>;
}

function CreateMeetingDialog({ open, onOpenChange, orgId }: { open: boolean; onOpenChange(open: boolean): void; orgId?: number }) { const [title, setTitle] = useState(""); const navigate = useNavigate(); const queryClient = useQueryClient(); const mutation = useMutation({ mutationFn: () => createRoom({ title }), onSuccess: (room) => { onOpenChange(false); void queryClient.invalidateQueries({ queryKey: ["organizations", orgId, "rooms"] }); navigate(`/meetings/${room.room.id}/preflight`); } }); return <Dialog.Root open={open} onOpenChange={onOpenChange}><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>创建会议</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>会议创建后可邀请组织成员加入。</Dialog.Description><FormError error={mutation.error} /><div className="form-stack"><label>会议标题<input className="field" value={title} onChange={(event) => setTitle(event.target.value)} /></label><button className="button-primary" disabled={!title.trim()} onClick={() => mutation.mutate()}>创建并预检</button></div></Dialog.Content></Dialog.Portal></Dialog.Root>; }

