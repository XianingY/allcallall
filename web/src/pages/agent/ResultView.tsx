export function ResultView({
  summary,
  risks = [],
  actions = [],
  next,
  status,
  runtimeLabel,
}: {
  summary?: string;
  risks?: string[];
  actions?: string[];
  next?: string;
  status?: string;
  runtimeLabel?: string;
}) {
  return (
    <>
      <header>
        <h2>结果</h2>
        <div className="result-badges">
          {runtimeLabel && (
            <span className="transcription-badge">{runtimeLabel}</span>
          )}
          <span className={`transcription-badge status-${status}`}>
            {status || "idle"}
          </span>
        </div>
      </header>
      <section>
        <h3>摘要</h3>
        <p>{summary || "等待运行结果"}</p>
      </section>
      <section>
        <h3>风险点</h3>
        {risks.map((item) => (
          <p key={item}>· {item}</p>
        ))}
      </section>
      <section>
        <h3>行动项</h3>
        {actions.map((item) => (
          <p key={item}>· {item}</p>
        ))}
      </section>
      {next && (
        <section>
          <h3>下一步</h3>
          <p>{next}</p>
        </section>
      )}
    </>
  );
}
