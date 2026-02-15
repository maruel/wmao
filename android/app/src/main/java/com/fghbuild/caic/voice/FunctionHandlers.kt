// Dispatches Gemini function calls to the caic API.
package com.fghbuild.caic.voice

import com.caic.sdk.ApiClient
import com.caic.sdk.CreateTaskReq
import com.caic.sdk.InputReq
import com.caic.sdk.RestartReq
import com.caic.sdk.SyncReq
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class FunctionHandlers(private val apiClient: ApiClient) {

    var onSetActiveTask: ((String) -> Unit)? = null

    suspend fun handle(name: String, args: JsonObject): JsonElement {
        return try {
            when (name) {
                "list_tasks" -> handleListTasks()
                "create_task" -> handleCreateTask(args)
                "get_task_detail" -> handleGetTaskDetail(args)
                "send_message" -> handleSendMessage(args)
                "answer_question" -> handleAnswerQuestion(args)
                "sync_task" -> handleSyncTask(args)
                "terminate_task" -> handleTerminateTask(args)
                "restart_task" -> handleRestartTask(args)
                "get_usage" -> handleGetUsage()
                "set_active_task" -> handleSetActiveTask(args)
                "list_repos" -> handleListRepos()
                else -> errorResult("Unknown function: $name")
            }
        } catch (@Suppress("TooGenericExceptionCaught") e: Exception) {
            errorResult("Error: ${e.message}")
        }
    }

    private suspend fun handleListTasks(): JsonElement {
        val tasks = apiClient.listTasks()
        val summaries = tasks.map { t ->
            JsonObject(
                mapOf(
                    "id" to JsonPrimitive(t.id),
                    "task" to JsonPrimitive(t.task.lines().first()),
                    "state" to JsonPrimitive(t.state),
                    "costUSD" to JsonPrimitive(t.costUSD),
                    "durationMs" to JsonPrimitive(t.durationMs),
                    "repo" to JsonPrimitive(t.repo),
                    "harness" to JsonPrimitive(t.harness),
                )
            )
        }
        return JsonObject(
            mapOf(
                "tasks" to JsonArray(summaries),
                "count" to JsonPrimitive(tasks.size),
            )
        )
    }

    private suspend fun handleCreateTask(args: JsonObject): JsonElement {
        val prompt = args.requireString("prompt")
        val repo = args.requireString("repo")
        val model = args.optString("model")
        val harness = args.optString("harness") ?: "claude"
        val resp = apiClient.createTask(
            CreateTaskReq(
                prompt = prompt,
                repo = repo,
                model = model,
                harness = harness,
            )
        )
        return JsonObject(
            mapOf(
                "status" to JsonPrimitive(resp.status),
                "id" to JsonPrimitive(resp.id),
            )
        )
    }

    private suspend fun handleGetTaskDetail(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val tasks = apiClient.listTasks()
        val task = tasks.find { it.id == taskId }
            ?: return errorResult("Task not found: $taskId")
        return JsonObject(
            mapOf(
                "id" to JsonPrimitive(task.id),
                "task" to JsonPrimitive(task.task),
                "state" to JsonPrimitive(task.state),
                "costUSD" to JsonPrimitive(task.costUSD),
                "durationMs" to JsonPrimitive(task.durationMs),
                "repo" to JsonPrimitive(task.repo),
                "branch" to JsonPrimitive(task.branch),
                "error" to JsonPrimitive(task.error ?: ""),
                "result" to JsonPrimitive(task.result ?: ""),
                "inputTokens" to JsonPrimitive(task.inputTokens),
                "outputTokens" to JsonPrimitive(task.outputTokens),
            )
        )
    }

    private suspend fun handleSendMessage(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val message = args.requireString("message")
        val resp = apiClient.sendInput(taskId, InputReq(prompt = message))
        return JsonObject(mapOf("status" to JsonPrimitive(resp.status)))
    }

    private suspend fun handleAnswerQuestion(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val answer = args.requireString("answer")
        val resp = apiClient.sendInput(taskId, InputReq(prompt = answer))
        return JsonObject(mapOf("status" to JsonPrimitive(resp.status)))
    }

    private suspend fun handleSyncTask(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val force = args["force"]?.jsonPrimitive?.booleanOrNull ?: false
        val resp = apiClient.syncTask(taskId, SyncReq(force = force))
        val result = mutableMapOf<String, JsonElement>(
            "status" to JsonPrimitive(resp.status),
        )
        resp.safetyIssues?.let { issues ->
            result["safetyIssues"] = JsonArray(
                issues.map { issue ->
                    JsonObject(
                        mapOf(
                            "file" to JsonPrimitive(issue.file),
                            "kind" to JsonPrimitive(issue.kind),
                            "detail" to JsonPrimitive(issue.detail),
                        )
                    )
                }
            )
        }
        return JsonObject(result)
    }

    private suspend fun handleTerminateTask(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val resp = apiClient.terminateTask(taskId)
        return JsonObject(mapOf("status" to JsonPrimitive(resp.status)))
    }

    private suspend fun handleRestartTask(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        val prompt = args.requireString("prompt")
        val resp = apiClient.restartTask(taskId, RestartReq(prompt = prompt))
        return JsonObject(mapOf("status" to JsonPrimitive(resp.status)))
    }

    private suspend fun handleGetUsage(): JsonElement {
        val usage = apiClient.getUsage()
        return JsonObject(
            mapOf(
                "fiveHour" to JsonObject(
                    mapOf(
                        "utilization" to JsonPrimitive(usage.fiveHour.utilization),
                        "resetsAt" to JsonPrimitive(usage.fiveHour.resetsAt),
                    )
                ),
                "sevenDay" to JsonObject(
                    mapOf(
                        "utilization" to JsonPrimitive(usage.sevenDay.utilization),
                        "resetsAt" to JsonPrimitive(usage.sevenDay.resetsAt),
                    )
                ),
            )
        )
    }

    @Suppress("UnusedPrivateMember")
    private fun handleSetActiveTask(args: JsonObject): JsonElement {
        val taskId = args.requireString("task_id")
        onSetActiveTask?.invoke(taskId)
        return JsonObject(mapOf("status" to JsonPrimitive("navigated")))
    }

    private suspend fun handleListRepos(): JsonElement {
        val repos = apiClient.listRepos()
        return JsonObject(
            mapOf(
                "repos" to JsonArray(
                    repos.map { r ->
                        JsonObject(
                            mapOf(
                                "path" to JsonPrimitive(r.path),
                                "baseBranch" to JsonPrimitive(r.baseBranch),
                            )
                        )
                    }
                ),
            )
        )
    }
}

private fun JsonObject.requireString(key: String): String =
    this[key]?.jsonPrimitive?.content
        ?: throw IllegalArgumentException("Missing required parameter: $key")

private fun JsonObject.optString(key: String): String? =
    this[key]?.jsonPrimitive?.content

private fun errorResult(message: String): JsonElement =
    JsonObject(mapOf("error" to JsonPrimitive(message)))
