interface Props { title: string }

export function PlaceholderPage({ title }: Props) {
  return <div className="page"><header className="page-header"><div><p className="eyebrow">AllCallAll Workspace</p><h1>{title}</h1></div></header><section className="empty-state"><p>该模块正在迁移到独立 Web 工作台。</p></section></div>;
}
