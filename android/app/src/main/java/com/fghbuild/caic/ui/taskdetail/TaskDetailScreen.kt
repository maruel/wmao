// Full-screen task detail view with live SSE message stream, grouping, and actions.
package com.fghbuild.caic.ui.taskdetail

import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.PlainTooltip
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TooltipBox
import androidx.compose.material3.TooltipDefaults
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.rememberTooltipState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.fghbuild.caic.ui.theme.stateColor
import com.fghbuild.caic.util.createCameraPhotoUri
import com.fghbuild.caic.util.uriToImageData

private val PlanBadgeBg = Color(0xFFEDE9FE)
private val PlanBadgeFg = Color(0xFF7C3AED)
private val TerminalStates = setOf("terminated", "failed")

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun TaskDetailScreen(
    taskId: String,
    onNavigateBack: () -> Unit,
    viewModel: TaskDetailViewModel = hiltViewModel(),
) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val task = state.task
    val context = LocalContext.current
    val contentResolver = context.contentResolver
    val photoPicker = rememberLauncherForActivityResult(
        ActivityResultContracts.PickMultipleVisualMedia(),
    ) { uris: List<Uri> ->
        val images = uris.mapNotNull { uriToImageData(contentResolver, it) }
        if (images.isNotEmpty()) viewModel.addImages(images)
    }
    var cameraUri by rememberSaveable { mutableStateOf<Uri?>(null) }
    val cameraLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.TakePicture(),
    ) { success: Boolean ->
        val uri = cameraUri
        if (success && uri != null) {
            val img = uriToImageData(contentResolver, uri)
            if (img != null) viewModel.addImages(listOf(img))
        }
        cameraUri = null
    }

    // Safety dialog
    if (state.safetyIssues.isNotEmpty()) {
        SafetyDialog(
            issues = state.safetyIssues,
            onDismiss = viewModel::dismissSafetyIssues,
            onForceSync = {
                viewModel.dismissSafetyIssues()
                viewModel.syncTask(force = true)
            },
        )
    }

    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Row(
                            horizontalArrangement = Arrangement.spacedBy(4.dp),
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                text = task?.repo ?: taskId,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis,
                            )
                            if (task?.inPlanMode == true) {
                                Surface(shape = RoundedCornerShape(4.dp), color = PlanBadgeBg) {
                                    Text(
                                        "P",
                                        style = MaterialTheme.typography.labelSmall,
                                        color = PlanBadgeFg,
                                        fontWeight = FontWeight.Bold,
                                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                                    )
                                }
                            }
                        }
                        task?.let {
                            Row(
                                horizontalArrangement = Arrangement.spacedBy(4.dp),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Text(
                                    text = it.branch,
                                    style = MaterialTheme.typography.bodySmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                                )
                                Surface(
                                    shape = RoundedCornerShape(4.dp),
                                    color = stateColor(it.state),
                                ) {
                                    Text(
                                        text = it.state,
                                        style = MaterialTheme.typography.labelSmall,
                                        modifier = Modifier.padding(horizontal = 4.dp, vertical = 1.dp),
                                    )
                                }
                            }
                        }
                    }
                },
                navigationIcon = {
                    TooltipBox(
                        positionProvider = TooltipDefaults.rememberPlainTooltipPositionProvider(),
                        tooltip = { PlainTooltip { Text("Back") } },
                        state = rememberTooltipState(),
                    ) {
                        IconButton(onClick = onNavigateBack) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                        }
                    }
                },
            )
        },
        bottomBar = {
            if (task?.state !in TerminalStates) {
                Box(modifier = Modifier.fillMaxWidth(), contentAlignment = Alignment.BottomCenter) {
                Column(modifier = Modifier.widthIn(max = 840.dp)) {
                    state.actionError?.let { error ->
                        Text(
                            text = error,
                            color = MaterialTheme.colorScheme.error,
                            style = MaterialTheme.typography.bodySmall,
                            modifier = Modifier.padding(horizontal = 12.dp, vertical = 2.dp),
                        )
                    }
                    InputBar(
                        draft = state.inputDraft,
                        onDraftChange = viewModel::updateInputDraft,
                        onSend = viewModel::sendInput,
                        onSync = { viewModel.syncTask() },
                        onTerminate = viewModel::terminateTask,
                        sending = state.sending,
                        pendingAction = state.pendingAction,
                        repoURL = task?.repoURL,
                        pendingImages = state.pendingImages,
                        supportsImages = state.supportsImages,
                        onAttachGallery = {
                            photoPicker.launch(
                                PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly)
                            )
                        },
                        onAttachCamera = {
                            val uri = createCameraPhotoUri(context)
                            cameraUri = uri
                            cameraLauncher.launch(uri)
                        },
                        onRemoveImage = viewModel::removeImage,
                    )
                }
                }
            }
        },
    ) { padding ->
        if (!state.isReady && !state.hasMessages) {
            Box(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(padding),
                contentAlignment = Alignment.Center,
            ) {
                CircularProgressIndicator()
            }
        } else {
            MessageList(
        state = state,
        padding = padding,
        onAnswer = { viewModel.sendInput() },
        onClearAndExecutePlan = {
            viewModel.restartTask(state.inputDraft.trim())
            viewModel.updateInputDraft("")
        },
    )
        }
    }
}

@Composable
private fun MessageList(
    state: TaskDetailState,
    padding: PaddingValues,
    onAnswer: (String) -> Unit,
    onClearAndExecutePlan: () -> Unit,
) {
    val listState = rememberLazyListState()
    var userScrolledUp by remember { mutableStateOf(false) }

    // Auto-scroll to bottom when new messages arrive, unless user scrolled up.
    LaunchedEffect(state.turns.size, state.messageCount) {
        if (!userScrolledUp && state.turns.isNotEmpty()) {
            val total = listState.layoutInfo.totalItemsCount
            val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1
            if (total > 0 && lastVisible < total - 1) {
                listState.animateScrollToItem(total - 1)
            }
        }
    }

    // Detect user scroll direction.
    LaunchedEffect(listState.isScrollInProgress) {
        if (listState.isScrollInProgress) {
            val info = listState.layoutInfo
            val lastVisible = info.visibleItemsInfo.lastOrNull()?.index ?: 0
            userScrolledUp = lastVisible < info.totalItemsCount - 2
        }
    }

    Box(modifier = Modifier.fillMaxSize().padding(padding), contentAlignment = Alignment.TopCenter) {
    Column(modifier = Modifier.widthIn(max = 840.dp).fillMaxWidth()) {
        // Todo panel
        if (state.todos.isNotEmpty()) {
            TodoPanel(
                todos = state.todos,
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 4.dp),
            )
        }

        // Message turns: past turns are elided; the last turn's groups are flattened into the
        // LazyColumn so that only visible groups are composed (avoids eagerly rendering 80+
        // Markdown composables for a turn with many tool-call commentary messages).
        SelectionContainer(modifier = Modifier.weight(1f)) {
            LazyColumn(
                state = listState,
                modifier = Modifier.fillMaxWidth(),
                contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                val turns = state.turns
                val lastTurn = turns.lastOrNull()
                val isWaiting = state.task?.state == "waiting"

                // Elided past turns.
                if (turns.size > 1) {
                    itemsIndexed(
                        items = turns.subList(0, turns.size - 1),
                        key = { _, turn ->
                            turn.groups.firstOrNull()?.events?.firstOrNull()?.ts ?: 0L
                        },
                    ) { _, turn ->
                        ElidedTurn(turn = turn)
                    }
                }

                // Last turn: groups are individual lazy items.
                if (lastTurn != null) {
                    itemsIndexed(
                        items = lastTurn.groups,
                        key = { _, group ->
                            "g:${group.events.firstOrNull()?.ts ?: 0L}"
                        },
                    ) { _, group ->
                        MessageGroupContent(
                            group = group,
                            turn = lastTurn,
                            onAnswer = onAnswer,
                            isWaiting = isWaiting,
                            onClearAndExecutePlan = onClearAndExecutePlan,
                        )
                    }
                }
            }
        }
    }
    }
}
