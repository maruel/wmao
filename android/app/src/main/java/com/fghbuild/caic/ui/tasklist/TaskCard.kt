// Composable card displaying a single task summary, mirroring TaskItemSummary.tsx.
package com.fghbuild.caic.ui.tasklist

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Card
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.caic.sdk.Harnesses
import com.caic.sdk.TaskJSON
import com.fghbuild.caic.ui.theme.stateColor
import com.fghbuild.caic.util.formatCost
import com.fghbuild.caic.util.formatElapsed

@Composable
fun TaskCard(task: TaskJSON, modifier: Modifier = Modifier) {
    val firstLine = task.task.lineSequence().firstOrNull().orEmpty()
    val supporting = buildString {
        append(task.repo)
        append(" \u2022 ")
        append(formatCost(task.costUSD))
        append(" \u2022 ")
        append(formatElapsed(task.durationMs))
    }

    Card(modifier = modifier) {
        ListItem(
            leadingContent = {
                Box(
                    modifier = Modifier
                        .size(12.dp)
                        .clip(CircleShape)
                        .background(stateColor(task.state))
                )
            },
            headlineContent = {
                Text(
                    text = firstLine,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            },
            supportingContent = {
                Text(
                    text = supporting,
                    style = MaterialTheme.typography.bodySmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            },
            trailingContent = {
                TrailingBadges(task)
            },
        )
    }
}

@Composable
private fun TrailingBadges(task: TaskJSON) {
    val badges = buildList {
        if (task.harness != Harnesses.Claude) {
            add(task.harness)
        }
        task.model?.let { add(it) }
    }
    if (badges.isNotEmpty()) {
        Text(
            text = badges.joinToString(" "),
            style = MaterialTheme.typography.labelSmall,
            maxLines = 1,
        )
    }
}
