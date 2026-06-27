import { AlertCircle, LoaderCircle } from "lucide-react";

export function PageLoading({ label = "正在加载" }: { label?: string }) {
  return <div className="page-state"><LoaderCircle className="animate-spin" size={20} />{label}</div>;
}

export function PageError({ error, retry }: { error: unknown; retry?: () => void }) {
  return <div className="page-state text-danger"><AlertCircle size={20} /><span>{error instanceof Error ? error.message : "加载失败"}</span>{retry && <button className="button-secondary" onClick={retry}>重试</button>}</div>;
}

