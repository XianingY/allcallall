import {
  Background,
  Controls,
  ReactFlow,
  type Edge,
  type Node,
} from "@xyflow/react";

import type { WorkflowTask } from "@/api/agent";

export function WorkflowGraph({ tasks }: { tasks: WorkflowTask[] }) {
  const nodes: Node[] = tasks.map((task, index) => ({
    id: String(task.id),
    position: { x: (index % 3) * 250, y: Math.floor(index / 3) * 130 },
    data: { label: `${task.name}\n${task.role} · ${task.status}` },
    className: `workflow-node status-${task.status}`,
  }));
  const byName = new Map(tasks.map((task) => [task.name, task]));
  const edges: Edge[] = tasks.flatMap((task) => {
    try {
      const deps = JSON.parse(task.depends_on_json || "[]") as string[];
      return deps
        .map((name) => byName.get(name))
        .filter(Boolean)
        .map((dependency) => ({
          id: `${dependency!.id}-${task.id}`,
          source: String(dependency!.id),
          target: String(task.id),
          animated: task.status === "running",
        }));
    } catch {
      return [];
    }
  });
  return (
    <div className="workflow-canvas">
      {nodes.length ? (
        <ReactFlow nodes={nodes} edges={edges} fitView>
          <Background />
          <Controls />
        </ReactFlow>
      ) : (
        <div className="pane-empty">选择一个 Workflow 查看任务图</div>
      )}
    </div>
  );
}
