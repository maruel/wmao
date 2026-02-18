// Bidirectional map between task IDs and stable 1-based human-friendly numbers.
package com.fghbuild.caic.voice

import com.caic.sdk.TaskJSON

class TaskNumberMap {
    private val idToNumber = mutableMapOf<String, Int>()
    private val numberToId = mutableMapOf<Int, String>()
    private var nextNumber = 1

    /** Sync with current task list. Existing tasks keep their number; new tasks get the next. */
    fun update(tasks: List<TaskJSON>) {
        val currentIds = tasks.map { it.id }.toSet()
        // Remove stale mappings.
        val stale = idToNumber.keys - currentIds
        for (id in stale) {
            val num = idToNumber.remove(id)
            if (num != null) numberToId.remove(num)
        }
        // Assign numbers to new tasks.
        for (task in tasks) {
            if (task.id !in idToNumber) {
                idToNumber[task.id] = nextNumber
                numberToId[nextNumber] = task.id
                nextNumber++
            }
        }
    }

    fun toId(number: Int): String? = numberToId[number]

    fun toNumber(id: String): Int? = idToNumber[id]

    fun formatTaskRef(task: TaskJSON): String {
        val num = idToNumber[task.id] ?: return task.id
        val shortName = task.task.lines().firstOrNull()?.take(SHORT_NAME_MAX) ?: task.id
        return "task #$num ($shortName)"
    }

    companion object {
        private const val SHORT_NAME_MAX = 40
    }
}
