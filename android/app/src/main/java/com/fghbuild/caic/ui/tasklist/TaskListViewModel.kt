// ViewModel for the task list screen, combining SSE task data with settings state.
package com.fghbuild.caic.ui.tasklist

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import com.caic.sdk.TaskJSON
import com.fghbuild.caic.data.TaskRepository
import com.fghbuild.caic.data.SettingsRepository
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import javax.inject.Inject

data class TaskListState(
    val tasks: List<TaskJSON> = emptyList(),
    val connected: Boolean = false,
    val serverConfigured: Boolean = false,
)

@HiltViewModel
class TaskListViewModel @Inject constructor(
    private val taskRepository: TaskRepository,
    private val settingsRepository: SettingsRepository,
) : ViewModel() {

    val state: StateFlow<TaskListState> = combine(
        taskRepository.tasks,
        taskRepository.connected,
        settingsRepository.settings,
    ) { tasks, connected, settings ->
        TaskListState(
            tasks = tasks,
            connected = connected,
            serverConfigured = settings.serverURL.isNotBlank(),
        )
    }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), TaskListState())

    init {
        taskRepository.start(viewModelScope)
    }
}
