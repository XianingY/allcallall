import { useQuery } from "@tanstack/react-query";
import { Clock3, FileAudio, HardDrive } from "lucide-react";
import { Link } from "react-router-dom";

import { listRecordings } from "@/api/meetings";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationProvider";

export function RecordingsPage() {
  const { activeOrganization } = useOrganization(); const query = useQuery({ queryKey: ["organizations", activeOrganization?.id, "recordings"], queryFn: listRecordings, enabled: Boolean(activeOrganization), refetchInterval: 10_000 });
  return <div className="page"><header className="page-header"><div><p className="eyebrow">Recordings</p><h1>录音与转写</h1><p>转写独立于实时翻译，在录制结束后自动处理。</p></div></header>{query.isLoading ? <PageLoading /> : query.isError ? <PageError error={query.error} /> : <div className="recording-list">{query.data?.map((recording) => <Link className="panel recording-row" key={recording.session.id} to={`/recordings/${recording.session.id}`}><div className="recording-icon"><FileAudio size={20} /></div><div><h2>会议录音 #{recording.session.id}</h2><p><Clock3 size={14} />{recording.session.started_at ? new Date(recording.session.started_at).toLocaleString() : "等待开始"}</p><p><HardDrive size={14} />{recording.files.length} 个文件</p></div><span className={`transcription-badge status-${recording.transcription?.status ?? "pending"}`}>{recording.transcription?.status ?? "pending"}</span></Link>)}</div>}</div>;
}

