// Floating voice mode overlay composable.
package com.fghbuild.caic.voice

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Mic
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

private const val PulseMinAlpha = 0.5f
private const val PulseMaxAlpha = 1.0f
private const val PulseDurationMs = 1000
private const val BarAnimDurationMs = 400
private const val BarCount = 3
private const val BarMinHeight = 4
private const val BarMaxHeight = 16
private const val OverlayCornerRadius = 16
private const val IndicatorSize = 12

@Composable
fun VoiceOverlay(
    voiceState: VoiceState,
    voiceEnabled: Boolean,
    onConnect: () -> Unit,
    onDisconnect: () -> Unit,
    modifier: Modifier = Modifier,
) {
    if (!voiceEnabled) return

    Box(
        modifier = modifier
            .fillMaxWidth()
            .padding(16.dp),
        contentAlignment = Alignment.BottomCenter,
    ) {
        when {
            voiceState.error != null -> ErrorMicButton(onConnect)
            voiceState.connectStatus != null -> ConnectingIndicator(voiceState.connectStatus)
            voiceState.listening || voiceState.speaking -> ActiveVoicePanel(
                voiceState = voiceState,
                onDisconnect = onDisconnect,
            )
            !voiceState.connected -> IdleMicButton(onConnect)
            // Connected but audio not yet started (brief transition).
            else -> ConnectingIndicator("Starting audio…")
        }
    }
}

@Composable
private fun IdleMicButton(onClick: () -> Unit) {
    FloatingActionButton(onClick = onClick) {
        Icon(Icons.Default.Mic, contentDescription = "Start voice")
    }
}

@Composable
private fun ConnectingIndicator(status: String) {
    val infiniteTransition = rememberInfiniteTransition(label = "pulse")
    val alpha by infiniteTransition.animateFloat(
        initialValue = PulseMinAlpha,
        targetValue = PulseMaxAlpha,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = PulseDurationMs),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "pulseAlpha",
    )
    Surface(
        shape = RoundedCornerShape(OverlayCornerRadius.dp),
        tonalElevation = 4.dp,
        shadowElevation = 4.dp,
        modifier = Modifier.alpha(alpha),
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Icon(Icons.Default.Mic, contentDescription = null)
            Text(
                text = status,
                style = MaterialTheme.typography.bodyMedium,
            )
        }
    }
}

@Composable
private fun ActiveVoicePanel(
    voiceState: VoiceState,
    onDisconnect: () -> Unit,
) {
    Surface(
        shape = RoundedCornerShape(OverlayCornerRadius.dp),
        tonalElevation = 4.dp,
        shadowElevation = 4.dp,
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 12.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            if (voiceState.speaking) {
                SpeakingIndicator()
            } else {
                ListeningIndicator()
            }

            val statusText = when {
                voiceState.activeTool != null -> voiceState.activeTool
                voiceState.speaking -> "Speaking..."
                else -> "Listening..."
            }
            Text(
                text = statusText,
                style = MaterialTheme.typography.bodyMedium,
                color = if (voiceState.activeTool != null) {
                    MaterialTheme.colorScheme.tertiary
                } else {
                    MaterialTheme.colorScheme.onSurface
                },
                modifier = Modifier.weight(1f),
            )

            IconButton(onClick = onDisconnect) {
                Icon(Icons.Default.Stop, contentDescription = "End voice")
            }
        }
    }
}

@Composable
private fun ListeningIndicator() {
    val infiniteTransition = rememberInfiniteTransition(label = "listening")
    val scale by infiniteTransition.animateFloat(
        initialValue = PulseMinAlpha,
        targetValue = PulseMaxAlpha,
        animationSpec = infiniteRepeatable(
            animation = tween(durationMillis = PulseDurationMs),
            repeatMode = RepeatMode.Reverse,
        ),
        label = "listeningPulse",
    )
    Box(
        modifier = Modifier
            .size(IndicatorSize.dp)
            .alpha(scale)
            .clip(CircleShape)
            .background(MaterialTheme.colorScheme.primary),
    )
}

@Composable
private fun SpeakingIndicator() {
    val infiniteTransition = rememberInfiniteTransition(label = "speaking")
    Row(
        horizontalArrangement = Arrangement.spacedBy(2.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        repeat(BarCount) { index ->
            val fraction by infiniteTransition.animateFloat(
                initialValue = 0f,
                targetValue = 1f,
                animationSpec = infiniteRepeatable(
                    animation = tween(
                        durationMillis = BarAnimDurationMs,
                        delayMillis = index * (BarAnimDurationMs / BarCount),
                    ),
                    repeatMode = RepeatMode.Reverse,
                ),
                label = "bar$index",
            )
            val heightTarget = BarMinHeight + fraction * (BarMaxHeight - BarMinHeight)
            val height by animateFloatAsState(
                targetValue = heightTarget,
                label = "barHeight$index",
            )
            Box(
                modifier = Modifier
                    .width(3.dp)
                    .height(height.dp)
                    .clip(RoundedCornerShape(1.dp))
                    .background(MaterialTheme.colorScheme.primary),
            )
        }
    }
}

@Composable
private fun ErrorMicButton(onClick: () -> Unit) {
    FloatingActionButton(
        onClick = onClick,
        containerColor = MaterialTheme.colorScheme.errorContainer,
    ) {
        Icon(
            Icons.Default.Mic,
            contentDescription = "Retry voice connection",
            tint = MaterialTheme.colorScheme.onErrorContainer,
        )
    }
}

@Suppress("UnusedPrivateMember")
@Composable
private fun StatusDot(color: Color) {
    Spacer(
        modifier = Modifier
            .size(IndicatorSize.dp)
            .clip(CircleShape)
            .background(color),
    )
}
