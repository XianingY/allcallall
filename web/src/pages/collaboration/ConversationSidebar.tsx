import { Search } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { PageEmpty, PageError, PageLoading } from "@/components/PageState";
import { formatTime } from "@/pages/collaboration/InboxFormat";
import { NewConversationDialog } from "@/pages/collaboration/InboxParts";
import type {
  ConversationFilterState,
  ConversationQueries,
} from "@/pages/collaboration/useConversationQueries";

interface ConversationSidebarProps {
  filter: ConversationFilterState;
  onFilterChange: (next: Partial<ConversationFilterState>) => void;
  queries: ConversationQueries;
  selectedId: number | null;
  organizationId?: number;
  onCreateConversation: () => void;
  onOpenConversation: (conversationId: number) => void;
}

export function ConversationSidebar({
  filter,
  onFilterChange,
  queries,
  selectedId,
  organizationId,
  onCreateConversation,
  onOpenConversation,
}: ConversationSidebarProps) {
  const [creating, setCreating] = useState(false);
  const { conversations, messageHits, visibleConversations } = queries;
  const openCreateDialog = () => {
    onCreateConversation();
    setCreating(true);
  };

  return (
    <aside className="conversation-list">
      <header className="workspace-pane-header">
        <div>
          <span className="eyebrow">Workspace</span>
          <h1>Inbox</h1>
        </div>
        <NewConversationDialog
          open={creating}
          onOpenChange={setCreating}
          orgId={organizationId}
          onCreated={onOpenConversation}
        />
      </header>

      <div className="search-field">
        <Search size={16} />
        <input
          aria-label="搜索会话，按回车搜索消息内容"
          placeholder="搜索会话；回车搜索消息内容"
          value={filter.keyword}
          onChange={(event) => {
            const next = event.target.value;
            onFilterChange({ keyword: next });
            if (!next.trim()) onFilterChange({ messageQuery: "" });
          }}
          onKeyDown={(event) => {
            if (event.key !== "Enter") return;
            event.preventDefault();
            onFilterChange({ messageQuery: filter.keyword.trim() });
          }}
        />
      </div>

      {filter.messageQuery ? (
        <div className="search-scope">
          <span>搜索消息内容：{filter.messageQuery}</span>
          <button className="button-secondary" onClick={() => onFilterChange({ messageQuery: "" })}>
            退出消息搜索
          </button>
        </div>
      ) : null}

      <div className="filter-tabs">
        <button
          className={!filter.status && !filter.unreadOnly ? "active" : ""}
          onClick={() => onFilterChange({ status: "", unreadOnly: false })}
        >
          全部
        </button>
        <button
          className={filter.unreadOnly ? "active" : ""}
          onClick={() => onFilterChange({ unreadOnly: true, status: "" })}
        >
          未读
        </button>
        <button
          className={filter.status === "my" ? "active" : ""}
          onClick={() => onFilterChange({ status: "my", unreadOnly: false })}
        >
          我的
        </button>
        <button
          className={filter.status === "open" ? "active" : ""}
          onClick={() => onFilterChange({ status: "open", unreadOnly: false })}
        >
          处理中
        </button>
        <button
          className={filter.status === "pending" ? "active" : ""}
          onClick={() => onFilterChange({ status: "pending", unreadOnly: false })}
        >
          待处理
        </button>
        <button
          className={filter.status === "resolved" ? "active" : ""}
          onClick={() => onFilterChange({ status: "resolved", unreadOnly: false })}
        >
          已解决
        </button>
        <button
          className={filter.status === "channels" ? "active" : ""}
          onClick={() => onFilterChange({ status: "channels", unreadOnly: false })}
        >
          频道
        </button>
      </div>

      {filter.messageQuery ? (
        messageHits.isLoading ? (
          <PageLoading label="正在搜索消息" />
        ) : messageHits.isError ? (
          <PageError error={messageHits.error} retry={() => void messageHits.refetch()} />
        ) : (messageHits.data?.length ?? 0) === 0 ? (
          <PageEmpty
            label="没有匹配的消息"
            hint={`“${filter.messageQuery}” 没有找到消息内容，换个关键词试试`}
          />
        ) : (
          <div className="conversation-items">
            {messageHits.data!.map((hit) => (
              <Link
                key={hit.id}
                to={`/conversations/${hit.conversation_id}`}
                className="conversation-item"
              >
                <div className="conversation-avatar">
                  {(hit.sender_display_name || hit.sender_email || "?").slice(0, 1).toUpperCase()}
                </div>
                <div className="conversation-copy">
                  <div>
                    <strong>{hit.sender_display_name || hit.sender_email || "未知发送者"}</strong>
                    <time>{formatTime(hit.created_at)}</time>
                  </div>
                  <p>{hit.body}</p>
                </div>
              </Link>
            ))}
          </div>
        )
      ) : null}

      {filter.messageQuery ? null : conversations.isLoading ? (
        <PageLoading />
      ) : conversations.isError ? (
        <PageError error={conversations.error} />
      ) : visibleConversations.length === 0 ? (
        <PageEmpty
          label={filter.keyword || filter.unreadOnly ? "没有匹配的会话" : "还没有会话"}
          hint={
            filter.keyword || filter.unreadOnly
              ? "换个关键词，或清除筛选条件"
              : "新建会话开始协作，或邀请联系人加入组织"
          }
          action={
            filter.keyword || filter.unreadOnly ? (
              <button
                className="button-secondary"
                onClick={() =>
                  onFilterChange({ keyword: "", unreadOnly: false, status: "" })
                }
              >
                清除筛选
              </button>
            ) : (
              <button className="button-secondary" onClick={openCreateDialog}>
                新建会话
              </button>
            )
          }
        />
      ) : (
        <div className="conversation-items">
          {visibleConversations.map((item) => (
            <Link
              key={item.id}
              to={`/conversations/${item.id}`}
              className={`conversation-item ${selectedId === item.id ? "conversation-item-active" : ""}`}
            >
              <div className="conversation-avatar">{item.title.slice(0, 1).toUpperCase()}</div>
              <div className="conversation-copy">
                <div>
                  <strong>{item.title}</strong>
                  <time>{formatTime(item.last_message_at)}</time>
                </div>
                <p>{item.last_message_preview || item.topic || "暂无消息"}</p>
                <span>{item.priority}</span>
              </div>
              {item.unread_count > 0 && <b className="unread-count">{item.unread_count}</b>}
            </Link>
          ))}
        </div>
      )}

      {conversations.data?.pages?.length ? (
        <div className="conversation-list-footer">
          <span>
            共{" "}
            {conversations.data.pages[conversations.data.pages.length - 1]?.pagination.total ?? 0}{" "}
            个会话
          </span>
          {conversations.hasNextPage ? (
            <button
              className="button-secondary"
              disabled={conversations.isFetchingNextPage}
              onClick={() => void conversations.fetchNextPage()}
            >
              加载更多
            </button>
          ) : null}
        </div>
      ) : null}
    </aside>
  );
}
