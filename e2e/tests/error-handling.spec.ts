// Error handling and edge case tests.
import { test, expect, createTaskAPI, waitForTaskState } from "../helpers";

test("POST /api/v1/tasks with missing prompt returns 400", async ({ request }) => {
  const res = await request.post("/api/v1/tasks", {
    data: { repo: "nonexistent", harness: "fake" },
  });
  expect(res.status()).toBe(400);
  const body = await res.json();
  expect(body.error.code).toBeTruthy();
});

test("POST /api/v1/tasks with missing repo returns 400", async ({ request }) => {
  const res = await request.post("/api/v1/tasks", {
    data: { prompt: "hello", harness: "fake" },
  });
  expect(res.status()).toBe(400);
});

test("POST /api/v1/tasks with unknown harness returns 400", async ({ request }) => {
  const res = await request.post("/api/v1/tasks", {
    data: { prompt: "hello", repo: "nonexistent", harness: "does-not-exist" },
  });
  expect(res.status()).toBe(400);
});

test("terminate nonexistent task returns 404", async ({ request }) => {
  const res = await request.post("/api/v1/tasks/nonexistent-id/terminate");
  expect(res.status()).toBe(404);
});

test("send input to nonexistent task returns 404", async ({ request }) => {
  const res = await request.post("/api/v1/tasks/nonexistent-id/input", {
    data: { prompt: "hello" },
  });
  expect(res.status()).toBe(404);
});

test("network failure shows reconnect banner", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByTestId("repo-select").locator("option")).not.toHaveCount(0);

  // Intercept all API requests to simulate network failure. This must close
  // the existing SSE connection too, so we abort any in-flight requests and
  // close the page's EventSource by navigating through a network-blocked state.
  await page.route("**/api/**", (route) => route.abort("failed"));

  // Force the existing SSE connection to break by evaluating a close on it,
  // then reload the page so it tries to reconnect through the blocked route.
  await page.reload();

  // The reconnecting banner has a 2s delay before appearing.
  await expect(page.getByTestId("reconnect-banner")).toBeVisible({
    timeout: 15_000,
  });

  // Restore network and reload to verify recovery.
  await page.unrouteAll({ behavior: "ignoreErrors" });
  await page.reload();
  await expect(page.getByTestId("reconnect-banner")).not.toBeVisible({
    timeout: 15_000,
  });
});

test("creating a task with special characters in prompt", async ({ api }) => {
  const prompt = '<script>alert("xss")</script> & "quotes" & émojis 🎉';
  const id = await createTaskAPI(api, prompt);
  await waitForTaskState(api, id, "waiting");

  const task = await api.getTask(id);
  expect(task).toBeTruthy();
  expect(task!.task).toBe(prompt);
});
