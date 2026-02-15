// Display formatting utilities for tasks: tokens, cost, and elapsed time.
package com.fghbuild.caic.util

import java.util.Locale

fun formatTokens(n: Int): String = when {
    n >= 1_000_000 -> "${n / 1_000_000}Mt"
    n >= 1_000 -> "${n / 1_000}kt"
    else -> "${n}t"
}

fun formatCost(usd: Double): String = when {
    usd < 0.01 -> "<$0.01"
    else -> "$${String.format(Locale.US, "%.2f", usd)}"
}

fun formatElapsed(ms: Long): String {
    val seconds = ms / 1000
    return when {
        seconds >= 3600 -> "${seconds / 3600}h ${(seconds % 3600) / 60}m"
        seconds >= 60 -> "${seconds / 60}m ${seconds % 60}s"
        else -> "${seconds}s"
    }
}
