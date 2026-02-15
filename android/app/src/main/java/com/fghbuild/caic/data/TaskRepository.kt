// Singleton repository managing the global SSE connection and task list state.
package com.fghbuild.caic.data

import com.caic.sdk.TaskJSON
import com.caic.sdk.UsageResp
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.onEach
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import java.io.IOException
import javax.inject.Inject
import javax.inject.Singleton

/** Discriminated union of SSE event types from the global events endpoint. */
sealed class GlobalEvent {
    data class Tasks(val tasks: List<TaskJSON>) : GlobalEvent()
    data class Usage(val usage: UsageResp) : GlobalEvent()
}

@Singleton
class TaskRepository @Inject constructor(
    private val settingsRepository: SettingsRepository,
) {
    private val _tasks = MutableStateFlow<List<TaskJSON>>(emptyList())
    val tasks: StateFlow<List<TaskJSON>> = _tasks.asStateFlow()

    private val _connected = MutableStateFlow(false)
    val connected: StateFlow<Boolean> = _connected.asStateFlow()

    private val client = OkHttpClient()
    private val json = Json { ignoreUnknownKeys = true }

    /**
     * Begins observing [SettingsRepository.settings]; restarts the SSE connection whenever the server URL changes.
     * Must be called once with a long-lived scope (e.g. viewModelScope).
     */
    fun start(scope: CoroutineScope) {
        scope.launch {
            settingsRepository.settings.collectLatest { settings ->
                if (settings.serverURL.isBlank()) {
                    _connected.value = false
                    _tasks.value = emptyList()
                    return@collectLatest
                }
                try {
                    globalEventsReconnecting(settings.serverURL).collect { event ->
                        _connected.value = true
                        when (event) {
                            is GlobalEvent.Tasks -> _tasks.value = event.tasks
                            is GlobalEvent.Usage -> { /* Usage updates not displayed yet. */ }
                        }
                    }
                } catch (e: CancellationException) {
                    throw e
                } catch (_: Exception) {
                    _connected.value = false
                }
            }
        }
    }

    /** Raw SSE flow for the global events endpoint. Emits one [GlobalEvent] per SSE message. */
    private fun globalEvents(baseURL: String): Flow<GlobalEvent> = callbackFlow {
        val request = Request.Builder()
            .url("$baseURL/api/v1/events")
            .header("Accept", "text/event-stream")
            .build()
        val factory = EventSources.createFactory(client)
        val source = factory.newEventSource(request, object : EventSourceListener() {
            override fun onEvent(eventSource: EventSource, id: String?, type: String?, data: String) {
                val event = when (type) {
                    "tasks" -> {
                        try {
                            GlobalEvent.Tasks(json.decodeFromString<List<TaskJSON>>(data))
                        } catch (_: Exception) {
                            null
                        }
                    }
                    "usage" -> {
                        try {
                            GlobalEvent.Usage(json.decodeFromString<UsageResp>(data))
                        } catch (_: Exception) {
                            null
                        }
                    }
                    else -> null
                }
                event?.let { trySend(it) }
            }

            override fun onFailure(eventSource: EventSource, t: Throwable?, response: Response?) {
                close(t?.let { IOException("SSE connection failed", it) })
            }

            override fun onClosed(eventSource: EventSource) {
                close()
            }
        })
        awaitClose { source.cancel() }
    }

    /** Reconnecting wrapper with exponential backoff (500ms initial, 1.5x, max 4s). */
    private fun globalEventsReconnecting(baseURL: String): Flow<GlobalEvent> = flow {
        var delayMs = 500L
        while (true) {
            try {
                globalEvents(baseURL).onEach { delayMs = 500L }.collect { emit(it) }
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                _connected.value = false
                delay(delayMs)
                delayMs = (delayMs * 3 / 2).coerceAtMost(4000L)
            }
        }
    }
}
