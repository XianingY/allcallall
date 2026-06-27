import * as Dialog from "@radix-ui/react-dialog";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Building2, MailPlus, Pencil, Plus, Trash2, X } from "lucide-react";
import { useState } from "react";

import { addContact, createInvitation, listContacts, removeContact, saveContactProfile, type Contact, type ContactProfile } from "@/api/collaboration";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function ContactsPage() {
  const { activeOrganization } = useOrganization(); const queryClient = useQueryClient(); const [email, setEmail] = useState(""); const [inviteEmail, setInviteEmail] = useState(""); const [inviteURL, setInviteURL] = useState("");
  const key = ["organizations", activeOrganization?.id, "contacts"];
  const contacts = useQuery({ queryKey: key, queryFn: listContacts, enabled: Boolean(activeOrganization) });
  const add = useMutation({ mutationFn: () => addContact(email), onSuccess: () => { setEmail(""); void queryClient.invalidateQueries({ queryKey: key }); } });
  const remove = useMutation({ mutationFn: removeContact, onSuccess: () => queryClient.invalidateQueries({ queryKey: key }) });
  const invite = useMutation({ mutationFn: () => createInvitation({ target_email: inviteEmail }), onSuccess: (value) => setInviteURL(value.share_url) });
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Relationships</p><h1>联系人</h1><p>维护联系人画像，并将上下文用于跟进和 Agent 工具。</p></div></header>
    <div className="toolbar-panel"><div className="search-inline"><input className="field" type="email" placeholder="通过邮箱添加联系人" value={email} onChange={(event) => setEmail(event.target.value)} /><button className="button-primary" disabled={!email} onClick={() => add.mutate()}><Plus size={17} />添加</button></div><Dialog.Root><Dialog.Trigger asChild><button className="button-secondary"><MailPlus size={17} />发送邀请</button></Dialog.Trigger><Dialog.Portal><Dialog.Overlay className="dialog-overlay" /><Dialog.Content className="dialog-content"><div className="dialog-header"><Dialog.Title>邀请联系人</Dialog.Title><Dialog.Close className="icon-button"><X size={18} /></Dialog.Close></div><Dialog.Description>生成可分享的邀请链接。</Dialog.Description><FormError error={invite.error} />{inviteURL ? <div className="status-success break-all">{inviteURL}</div> : <div className="form-stack"><label>邮箱<input className="field" type="email" value={inviteEmail} onChange={(event) => setInviteEmail(event.target.value)} /></label><button className="button-primary" onClick={() => invite.mutate()}>生成邀请</button></div>}</Dialog.Content></Dialog.Portal></Dialog.Root></div>
    <FormError error={add.error || remove.error} />
    {contacts.isLoading ? <PageLoading /> : contacts.isError ? <PageError error={contacts.error} /> : contacts.data?.length ? <div className="contact-grid">{contacts.data.map((contact) => <ContactCard key={contact.id} contact={contact} queryKey={key} onRemove={() => remove.mutate(contact.id)} />)}</div> : <div className="empty-state">暂无联系人</div>}
  </div>;
}

function ContactCard({ contact, queryKey, onRemove }: { contact: Contact; queryKey: unknown[]; onRemove(): void }) {
  const queryClient = useQueryClient(); const [editing, setEditing] = useState(false); const [profile, setProfile] = useState<ContactProfile>(contact.profile ?? {});
  const save = useMutation({ mutationFn: () => saveContactProfile(contact.id, profile), onSuccess: () => { setEditing(false); void queryClient.invalidateQueries({ queryKey }); } });
  return <article className="panel contact-card"><header><div className="contact-avatar">{contact.display_name.slice(0, 1).toUpperCase()}</div><div><h2>{contact.display_name}</h2><p>{contact.email}</p></div><div className="card-actions"><button className="icon-button" title="编辑画像" onClick={() => setEditing(!editing)}><Pencil size={16} /></button><button className="icon-button text-danger" title="移除联系人" onClick={onRemove}><Trash2 size={16} /></button></div></header>{editing ? <div className="form-stack compact"><FormError error={save.error} /><label>公司<input className="field" value={profile.company ?? ""} onChange={(event) => setProfile({ ...profile, company: event.target.value })} /></label><label>角色<input className="field" value={profile.role ?? ""} onChange={(event) => setProfile({ ...profile, role: event.target.value })} /></label><label>时区<input className="field" value={profile.timezone ?? ""} onChange={(event) => setProfile({ ...profile, timezone: event.target.value })} /></label><label>关系状态<input className="field" value={profile.relationship_status ?? ""} onChange={(event) => setProfile({ ...profile, relationship_status: event.target.value })} /></label><label>备注<textarea className="field" value={profile.note ?? ""} onChange={(event) => setProfile({ ...profile, note: event.target.value })} /></label><button className="button-primary" onClick={() => save.mutate()}>保存画像</button></div> : <dl className="contact-meta"><div><dt><Building2 size={14} />公司</dt><dd>{profile.company || "-"}</dd></div><div><dt>角色</dt><dd>{profile.role || "-"}</dd></div><div><dt>关系</dt><dd>{profile.relationship_status || "-"}</dd></div><div><dt>备注</dt><dd>{profile.note || "-"}</dd></div></dl>}</article>;
}
