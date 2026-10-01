import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronDown, ChevronRight, PhoneCall, Sparkles } from "lucide-react";
import { Fragment, useState } from "react";

import { fetchCallFollowup, generateCallFollowup, listCallHistory } from "@/api/collaboration";
import { formatDateTime } from "@allcallall/shared";
import { PageEmpty, PageError, PageLoading } from "@/components/PageState";

export function CallHistoryPage() {
  const queryClient = useQueryClient();
  const [openCallId, setOpenCallId] = useState<string | null>(null);
  const query = useQuery({ queryKey: ["calls", "history", 30], queryFn: () => listCallHistory(30) });

  // Fetched lazily: opening a row is what asks for its summary, so a history
  // page does not pull one record per call.
  const followup = useQuery({
    queryKey: ["calls", openCallId, "followup"],
    queryFn: () => fetchCallFollowup(openCallId!),
    enabled: Boolean(openCallId),
    retry: false,
  });
  const generate = useMutation({
    mutationFn: (force: boolean) => generateCallFollowup(openCallId!, force),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["calls", openCallId, "followup"] }),
  });

  return (
    <div className="page">
      <header className="page-header">
        <div>
          <p className="eyebrow">Direct calls</p>
          <h1>通话历史</h1>
          <p>最近 30 天的一对一通话、会后摘要与跟进状态。</p>
        </div>
      </header>
      {query.isLoading ? (
        <PageLoading />
      ) : query.isError ? (
        <PageError error={query.error} retry={() => void query.refetch()} />
      ) : (query.data?.length ?? 0) === 0 ? (
        <PageEmpty label="最近 30 天没有通话记录" hint="发起一对一通话后，记录会出现在这里" />
      ) : (
        <div className="panel table-wrap">
          <table>
            <thead>
              <tr>
                <th />
                <th>参与者</th>
                <th>开始时间</th>
                <th>状态</th>
                <th>结束原因</th>
                <th>跟进</th>
              </tr>
            </thead>
            <tbody>
              {query.data?.map((call) => {
                const callId = String(call.id);
                const isOpen = openCallId === callId;
                const record = followup.data?.followup;
                return (
                  <Fragment key={callId}>
                    <tr>
                      <td>
                        <button
                          className="icon-button"
                          aria-label={isOpen ? "收起摘要" : "展开摘要"}
                          aria-expanded={isOpen}
                          onClick={() => setOpenCallId(isOpen ? null : callId)}
                        >
                          {isOpen ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                        </button>
                      </td>
                      <td>
                        <span className="table-primary">
                          <PhoneCall size={15} />
                          {call.caller_display_name} / {call.callee_display_name}
                        </span>
                      </td>
                      {/* Shared formatter: this page used toLocaleString, so the
                          same call rendered differently on web and mobile. */}
                      <td>{formatDateTime(call.started_at)}</td>
                      <td>{call.status}</td>
                      <td>{call.end_reason || "-"}</td>
                      <td>{call.followup_status || "-"}</td>
                    </tr>
                    {isOpen ? (
                      <tr>
                        <td colSpan={6}>
                          {followup.isLoading ? (
                            <PageLoading label="正在加载摘要" />
                          ) : followup.isError ? (
                            <div className="call-summary-empty">
                              <p>这次通话还没有会后摘要。</p>
                              <button
                                className="button-primary"
                                disabled={generate.isPending}
                                onClick={() => generate.mutate(false)}
                              >
                                <Sparkles size={16} />
                                {generate.isPending ? "生成中…" : "生成摘要"}
                              </button>
                              {generate.isError ? (
                                <p className="text-danger" role="alert">
                                  生成失败：{generate.error instanceof Error ? generate.error.message : "请稍后重试"}
                                </p>
                              ) : null}
                            </div>
                          ) : (
                            <div className="call-summary">
                              <p>{record?.summary_cn || record?.summary_en || "摘要为空"}</p>
                              {record?.key_points?.length ? (
                                <div>
                                  <strong>要点</strong>
                                  <ul>
                                    {record.key_points.map((point) => (
                                      <li key={point}>{point}</li>
                                    ))}
                                  </ul>
                                </div>
                              ) : null}
                              {record?.action_items?.length ? (
                                <div>
                                  <strong>行动项</strong>
                                  <ul>
                                    {record.action_items.map((item) => (
                                      <li key={item}>{item}</li>
                                    ))}
                                  </ul>
                                </div>
                              ) : null}
                              {record?.next_step ? <p>下一步：{record.next_step}</p> : null}
                              <button
                                className="button-secondary"
                                disabled={generate.isPending}
                                onClick={() => generate.mutate(true)}
                              >
                                <Sparkles size={15} />
                                {generate.isPending ? "重新生成中…" : "重新生成"}
                              </button>
                            </div>
                          )}
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
