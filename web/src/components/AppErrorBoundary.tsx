import React from "react";

interface State { error: Error | null }

export class AppErrorBoundary extends React.Component<React.PropsWithChildren, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State { return { error }; }

  render() {
    if (this.state.error) {
      return (
        <main className="grid min-h-screen place-items-center bg-canvas p-6">
          <section className="w-full max-w-lg rounded-lg border border-line bg-panel p-6 shadow-panel">
            <h1 className="text-xl font-semibold text-ink">页面加载失败</h1>
            <p className="mt-2 text-sm text-muted">{this.state.error.message}</p>
            <button className="button-primary mt-5" onClick={() => window.location.reload()}>重新加载</button>
          </section>
        </main>
      );
    }
    return this.props.children;
  }
}
