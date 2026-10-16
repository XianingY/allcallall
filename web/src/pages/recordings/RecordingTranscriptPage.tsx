import * as Dialog from "@radix-ui/react-dialog";
import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Download, History, Pencil, RefreshCw, X } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import {
  downloadRecording,
  editTranscriptSegment,
  getRecording,
  getTranscript,
  listTranscriptSegmentRevisions,
  retryTranscription,
  type TranscriptSegment,
} from "@/api/meetings";
import { useAuth } from "@/auth/AuthContext";
import { FormError } from "@/components/AuthLayout";
import { PageError, PageLoading } from "@/components/PageState";
import { useOrganization } from "@/organizations/OrganizationContext";

const clock = (ms: number) => `${Math.floor(ms / 60000).toString().padStart(2, "0")}:${Math.floor((ms % 60000) / 1000).toString().padStart(2, "0")}`;

type TextDiff = { prefix: string; removed: string; added: string; suffix: string };

function textDiff(before: string, after: string): TextDiff {
  let start = 0;
  const min = Math.min(before.length, after.length);
  while (start < min && before[start] === after[start]) start += 1;
  let endBefore = before.length;
  let endAfter = after.length;
  while (endBefore > start && endAfter > start && before[endBefore - 1] === after[endAfter - 1]) {
    endBefore -= 1;
    endAfter -= 1;
  }
  return {
    prefix: before.slice(0, start),
    removed: before.slice(start, endBefore),
    added: after.slice(start, endAfter),
    suffix: after.slice(endAfter),
  };
}

function DiffText({ diff }: { diff: TextDiff }) {
  return (
    <p className="transcript-diff" data-testid="transcript-diff">
      <span>{diff.prefix}</span>
      {diff.removed ? <del className="diff-removed">{diff.removed}</del> : null}
      {diff.added ? <ins className="diff-added">{diff.added}</ins> : null}
      <span>{diff.suffix}</span>
    </p>
  );
}

export function RecordingTranscriptPage() {
  const id = Number(useParams().recordingId);
  const [params] = useSearchParams();
  const targetId = params.get("segmentId");
  const { user } = useAuth();
  const { activeOrganization } = useOrganization();
  const queryClient = useQueryClient();
  const recording = useQuery({
    queryKey: ["recordings", id],
    queryFn: () => getRecording(id),
    enabled: Boolean(id),
    refetchInterval: (query) => (["pending", "processing"].includes(query.state.data?.transcription?.status ?? "") ? 5000 : false),
  });
  const transcript = useInfiniteQuery({
    queryKey: ["recordings", id, "transcript"],
    queryFn: ({ pageParam }) => getTranscript(id, pageParam),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (page) => page.next_after_id ?? undefined,
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const status = query.state.data?.pages[0]?.transcription?.status;
      return status === "pending" || status === "processing" ? 5000 : false;
    },
  });
  const retry = useMutation({ mutationFn: () => retryTranscription(id), onSuccess: () => { void recording.refetch(); void transcript.refetch(); } });
  const segments = transcript.data?.pages.flatMap((page) => page.segments) ?? [];
  const status = transcript.data?.pages[0]?.transcription ?? recording.data?.transcription;
  const ready = status?.status === "ready";
  const role = activeOrganization?.role;
  const canEditSegment = (segment: TranscriptSegment) => Boolean(user) && ready && (role === "owner" || role === "admin" || segment.speaker_user_id === user?.id);
  useEffect(() => { if (!targetId) return; document.getElementById(`segment-${targetId}`)?.scrollIntoView({ block: "center" }); }, [targetId, segments.length]);
  const [downloadError, setDownloadError] = useState<unknown>();
  const download = async (fileId: number, fallbackName: string) => {
    try {
      setDownloadError(undefined);
      const result = await downloadRecording(id, fileId);
      const url = URL.createObjectURL(result.blob);
      const anchor = document.createElement("a");
      anchor.href = url;
      anchor.download = result.fileName ?? fallbackName;
      anchor.click();
      URL.revokeObjectURL(url);
    } catch (caught) {
      setDownloadError(caught);
    }
  };
  const [editing, setEditing] = useState<TranscriptSegment | null>(null);
  const [historySegment, setHistorySegment] = useState<TranscriptSegment | null>(null);
  if (recording.isLoading) return <PageLoading />;
  if (recording.isError || !recording.data) return <PageError error={recording.error ?? new Error("录音不存在")} />;
  return (
    <div className="page transcript-page">
      <header className="page-header">
        <div>
          <Link className="back-link" to="/recordings"><ArrowLeft size={16} />返回录音列表</Link>
          <h1>会议转写 #{id}</h1>
          <p>{status?.provider ? `Provider: ${status.provider}` : "等待转写 Provider"}</p>
          {downloadError ? <p className="status-error" role="alert">下载失败：{downloadError instanceof Error ? downloadError.message : String(downloadError)}</p> : null}
        </div>
        <div className="button-row">
          {(recording.data.files ?? []).map((file) => (
            <button className="button-secondary" key={file.id} onClick={() => void download(file.id, file.file_name)}><Download size={16} />{file.file_name}</button>
          ))}
        </div>
      </header>
      <section className="transcript-status">
        <span className={`transcription-badge status-${status?.status ?? "pending"}`}>{status?.status ?? "pending"}</span>
        <strong>{status?.segment_count ?? segments.length} 个片段</strong>
        {status?.error_message && <p>{status.error_message}</p>}
        {status?.status === "failed" && <button className="button-secondary" onClick={() => retry.mutate()}><RefreshCw size={16} />重试转写</button>}
      </section>
      {transcript.isLoading ? <PageLoading label="正在加载转写" /> : null}
      {transcript.isError ? <PageError error={transcript.error} retry={() => void transcript.refetch()} /> : null}
      {!transcript.isLoading && !transcript.isError && segments.length ? (
        <div className="transcript-timeline">
          {segments.map((segment) => (
            <article id={`segment-${segment.id}`} className={targetId === String(segment.id) ? "target" : ""} key={segment.id}>
              <time>{clock(segment.start_ms)}</time>
              <div>
                <header>
                  <strong>{segment.track_key || (segment.speaker_user_id ? `参会人 ${segment.speaker_user_id}` : "未知说话人")}</strong>
                  <span>{segment.language || "-"} · {Math.round(segment.confidence * 100)}%{segment.edited_at ? " · 已编辑" : ""}</span>
                  {segment.edited_at ? <button aria-label={`修订历史 ${segment.id}`} className="icon-button" onClick={() => setHistorySegment(segment)}><History size={14} /></button> : null}
                  {canEditSegment(segment) ? <button aria-label={`编辑片段 ${segment.id}`} className="icon-button" onClick={() => setEditing(segment)}><Pencil size={14} /></button> : null}
                </header>
                <p>{segment.text}</p>
              </div>
            </article>
          ))}
        </div>
      ) : null}
      {!transcript.isLoading && !transcript.isError && !segments.length ? (
        <div className="empty-state">
          {status?.status === "failed" ? "转写失败，可由管理员重试" : status?.status === "processing" ? "录音转写处理中" : status?.status === "skipped" ? "该录音未绑定会话，已跳过转写" : "尚无录音转写"}
        </div>
      ) : null}
      {transcript.hasNextPage ? <button className="button-secondary mx-auto mt-4 flex" disabled={transcript.isFetchingNextPage} onClick={() => void transcript.fetchNextPage()}>{transcript.isFetchingNextPage ? "加载中…" : "加载更多"}</button> : null}
      <EditSegmentDialog key={editing ? `edit-${editing.id}` : "closed"} recordingId={id} segment={editing} onClose={() => setEditing(null)} onSaved={() => { setEditing(null); void queryClient.invalidateQueries({ queryKey: ["recordings", id, "transcript"] }); }} />
      <RevisionHistoryDialog recordingId={id} segment={historySegment} onClose={() => setHistorySegment(null)} />
    </div>
  );
}

function EditSegmentDialog({ recordingId, segment, onClose, onSaved }: { recordingId: number; segment: TranscriptSegment | null; onClose(): void; onSaved(): void }) {
  const [text, setText] = useState(() => segment?.text ?? "");
  const [reason, setReason] = useState("");
  const mutation = useMutation({
    mutationFn: (input: { segment: TranscriptSegment; text: string; reason: string }) => editTranscriptSegment(recordingId, input.segment.id, { text: input.text, reason: input.reason || undefined }),
    onSuccess: onSaved,
  });
  if (!segment) return null;
  const diff = textDiff(segment.text, text);
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content">
          <div className="dialog-header">
            <Dialog.Title>编辑转写片段</Dialog.Title>
            <Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close>
          </div>
          <Dialog.Description>修正 provider 的识别错误。每次修改都会保存修订历史。</Dialog.Description>
          <FormError error={mutation.error} />
          <div className="form-stack">
            <label>原文<DiffText diff={textDiff(segment.text, segment.text)} /></label>
            <label>修改后
              <textarea aria-label="修改后文本" className="field" maxLength={4096} rows={4} value={text} onChange={(event) => setText(event.target.value)} />
            </label>
            {text !== segment.text ? <label>差异预览<DiffText diff={diff} /></label> : null}
            <label>修订理由（可选）<input aria-label="修订理由" className="field" maxLength={512} value={reason} onChange={(event) => setReason(event.target.value)} /></label>
            <div className="button-row">
              <button className="button-secondary" onClick={onClose}>取消</button>
              <button className="button-primary" disabled={!text.trim() || text === segment.text || mutation.isPending} onClick={() => mutation.mutate({ segment, text, reason })}>保存修订</button>
            </div>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}

function RevisionHistoryDialog({ recordingId, segment, onClose }: { recordingId: number; segment: TranscriptSegment | null; onClose(): void }) {
  const revisions = useInfiniteQuery({
    queryKey: ["recordings", recordingId, "transcript-revisions", segment?.id],
    queryFn: ({ pageParam }) => listTranscriptSegmentRevisions(recordingId, segment!.id, pageParam),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (page) => page.next_before_id ?? undefined,
    enabled: Boolean(segment),
  });
  if (!segment) return null;
  const items = revisions.data?.pages.flatMap((page) => page.revisions) ?? [];
  return (
    <Dialog.Root open onOpenChange={(open) => { if (!open) onClose(); }}>
      <Dialog.Portal>
        <Dialog.Overlay className="dialog-overlay" />
        <Dialog.Content className="dialog-content">
          <div className="dialog-header">
            <Dialog.Title>片段 #{segment.id} 修订历史</Dialog.Title>
            <Dialog.Close className="icon-button" aria-label="关闭"><X size={18} /></Dialog.Close>
          </div>
          {revisions.isLoading ? <PageLoading label="正在加载修订历史" /> : null}
          {revisions.isError ? <PageError error={revisions.error} retry={() => void revisions.refetch()} /> : null}
          {!revisions.isLoading && !revisions.isError && items.length ? (
            <div className="revision-list">
              {items.map((revision) => (
                <article className="revision-item" key={revision.id}>
                  <header>
                    <strong>修订 {revision.revision_number}</strong>
                    <span>{new Date(revision.created_at).toLocaleString()}</span>
                    {revision.superseded ? <span className="revision-superseded">已被重跑取代</span> : null}
                  </header>
                  <DiffText diff={textDiff(revision.previous_text, revision.next_text)} />
                  {revision.reason ? <p className="revision-reason">{revision.reason}</p> : null}
                </article>
              ))}
              {revisions.hasNextPage ? <button className="button-secondary" disabled={revisions.isFetchingNextPage} onClick={() => void revisions.fetchNextPage()}>{revisions.isFetchingNextPage ? "加载中…" : "加载更多"}</button> : null}
            </div>
          ) : null}
          {!revisions.isLoading && !revisions.isError && !items.length ? <p>该片段还没有修订记录。</p> : null}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
