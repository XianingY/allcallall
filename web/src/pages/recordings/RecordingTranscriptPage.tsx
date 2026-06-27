import { useInfiniteQuery, useMutation, useQuery } from "@tanstack/react-query";
import { ArrowLeft, Download, RefreshCw } from "lucide-react";
import { useEffect } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import { downloadRecording, getRecording, getTranscript, retryTranscription } from "@/api/meetings";
import { PageError, PageLoading } from "@/components/PageState";

const clock = (ms: number) => `${Math.floor(ms / 60000).toString().padStart(2, "0")}:${Math.floor((ms % 60000) / 1000).toString().padStart(2, "0")}`;

export function RecordingTranscriptPage() {
  const id = Number(useParams().recordingId); const [params] = useSearchParams(); const targetId = params.get("segmentId");
  const recording = useQuery({ queryKey: ["recordings", id], queryFn: () => getRecording(id), enabled: Boolean(id), refetchInterval: (query) => ["pending", "processing"].includes(query.state.data?.transcription?.status ?? "") ? 5000 : false });
  const transcript = useInfiniteQuery({ queryKey: ["recordings", id, "transcript"], queryFn: ({ pageParam }) => getTranscript(id, pageParam), initialPageParam: undefined as number | undefined, getNextPageParam: (page) => page.next_after_id ?? undefined, enabled: Boolean(id), refetchInterval: (query) => { const status = query.state.data?.pages[0]?.transcription?.status; return status === "pending" || status === "processing" ? 5000 : false; } });
  const retry = useMutation({ mutationFn: () => retryTranscription(id), onSuccess: () => { void recording.refetch(); void transcript.refetch(); } });
  const segments = transcript.data?.pages.flatMap((page) => page.segments) ?? []; const status = transcript.data?.pages[0]?.transcription ?? recording.data?.transcription;
  useEffect(() => { if (!targetId) return; document.getElementById(`segment-${targetId}`)?.scrollIntoView({ block: "center" }); }, [targetId, segments.length]);
  const download = async (fileId: number, fallbackName: string) => { const result = await downloadRecording(id, fileId); const url = URL.createObjectURL(result.blob); const anchor = document.createElement("a"); anchor.href = url; anchor.download = result.fileName ?? fallbackName; anchor.click(); URL.revokeObjectURL(url); };
  if (recording.isLoading) return <PageLoading />; if (recording.isError || !recording.data) return <PageError error={recording.error ?? new Error("录音不存在")} />;
  return <div className="page transcript-page"><header className="page-header"><div><Link className="back-link" to="/recordings"><ArrowLeft size={16} />返回录音列表</Link><h1>会议转写 #{id}</h1><p>{status?.provider ? `Provider: ${status.provider}` : "等待转写 Provider"}</p></div><div className="button-row">{recording.data.files.map((file) => <button className="button-secondary" key={file.id} onClick={() => void download(file.id, file.file_name)}><Download size={16} />{file.file_name}</button>)}</div></header><section className="transcript-status"><span className={`transcription-badge status-${status?.status ?? "pending"}`}>{status?.status ?? "pending"}</span><strong>{status?.segment_count ?? segments.length} 个片段</strong>{status?.error_message && <p>{status.error_message}</p>}{status?.status === "failed" && <button className="button-secondary" onClick={() => retry.mutate()}><RefreshCw size={16} />重试转写</button>}</section>
    {transcript.isLoading ? <PageLoading label="正在加载转写" /> : transcript.isError ? <PageError error={transcript.error} retry={() => void transcript.refetch()} /> : segments.length ? <div className="transcript-timeline">{segments.map((segment) => <article id={`segment-${segment.id}`} className={targetId === String(segment.id) ? "target" : ""} key={segment.id}><time>{clock(segment.start_ms)}</time><div><header><strong>{segment.track_key || (segment.speaker_user_id ? `参会人 ${segment.speaker_user_id}` : "未知说话人")}</strong><span>{segment.language || "-"} · {Math.round(segment.confidence * 100)}%</span></header><p>{segment.text}</p></div></article>)}</div> : <div className="empty-state">{status?.status === "failed" ? "转写失败，可由管理员重试" : status?.status === "processing" ? "录音转写处理中" : status?.status === "skipped" ? "该录音未绑定会话，已跳过转写" : "尚无录音转写"}</div>}
    {transcript.hasNextPage && <button className="button-secondary mx-auto mt-4 flex" onClick={() => void transcript.fetchNextPage()}>加载更多</button>}
  </div>;
}
