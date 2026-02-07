import { createSignal, For, Show } from "solid-js";

interface TaskResult {
  task: string;
  branch: string;
  state: string;
  diffStat: string;
  costUSD: number;
  durationMs: number;
  numTurns: number;
  error?: string;
}

export default function App() {
  const [prompt, setPrompt] = createSignal("");
  const [tasks, setTasks] = createSignal<TaskResult[]>([]);
  const [submitting, setSubmitting] = createSignal(false);

  async function submitTask() {
    const p = prompt().trim();
    if (!p) return;
    setSubmitting(true);
    try {
      const res = await fetch("/api/tasks", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ prompt: p }),
      });
      if (!res.ok) throw new Error(await res.text());
      setPrompt("");
      await refreshTasks();
    } finally {
      setSubmitting(false);
    }
  }

  async function refreshTasks() {
    const res = await fetch("/api/tasks");
    if (res.ok) {
      setTasks(await res.json());
    }
  }

  // Poll for updates.
  setInterval(refreshTasks, 5000);
  refreshTasks();

  return (
    <div style={{ "max-width": "900px", margin: "0 auto", padding: "2rem", "font-family": "system-ui" }}>
      <h1>wmao</h1>
      <p>Work my ass off. Manage coding agents.</p>

      <form onSubmit={(e) => { e.preventDefault(); submitTask(); }}
        style={{ display: "flex", gap: "0.5rem", "margin-bottom": "2rem" }}>
        <input
          type="text"
          value={prompt()}
          onInput={(e) => setPrompt(e.currentTarget.value)}
          placeholder="Describe a task..."
          disabled={submitting()}
          style={{ flex: 1, padding: "0.5rem" }}
        />
        <button type="submit" disabled={submitting() || !prompt().trim()}>
          {submitting() ? "Submitting..." : "Run"}
        </button>
      </form>

      <h2>Tasks</h2>
      <Show when={tasks().length === 0}>
        <p style={{ color: "#888" }}>No tasks yet.</p>
      </Show>
      <For each={tasks()}>
        {(t) => (
          <div style={{
            border: "1px solid #ddd", "border-radius": "6px",
            padding: "1rem", "margin-bottom": "0.75rem",
          }}>
            <div style={{ display: "flex", "justify-content": "space-between" }}>
              <strong>{t.task}</strong>
              <span style={{
                padding: "0.15rem 0.5rem", "border-radius": "4px", "font-size": "0.85rem",
                background: t.state === "done" ? "#d4edda" : t.state === "failed" ? "#f8d7da" : "#fff3cd",
              }}>
                {t.state}
              </span>
            </div>
            <Show when={t.branch}>
              <div style={{ "font-size": "0.85rem", color: "#555" }}>branch: {t.branch}</div>
            </Show>
            <Show when={t.diffStat}>
              <pre style={{ "font-size": "0.8rem", background: "#f8f9fa", padding: "0.5rem" }}>{t.diffStat}</pre>
            </Show>
            <Show when={t.error}>
              <div style={{ color: "red", "font-size": "0.85rem" }}>{t.error}</div>
            </Show>
            <Show when={t.costUSD > 0}>
              <span style={{ "font-size": "0.8rem", color: "#888" }}>
                ${t.costUSD.toFixed(4)} &middot; {(t.durationMs / 1000).toFixed(1)}s &middot; {t.numTurns} turns
              </span>
            </Show>
          </div>
        )}
      </For>
    </div>
  );
}
