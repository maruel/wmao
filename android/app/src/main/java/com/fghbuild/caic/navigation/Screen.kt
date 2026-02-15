// Navigation graph for the app.
package com.fghbuild.caic.navigation

sealed class Screen(val route: String) {
    data object TaskList : Screen("tasks")
    data object Settings : Screen("settings")
}
