package org.fictionally.ringpromoter.ui

import android.net.Uri
import androidx.activity.compose.LocalActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material.icons.filled.History
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavDestination.Companion.hierarchy
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.LocalContainer
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.activity.ActivityScreen
import org.fictionally.ringpromoter.ui.activity.ActivityViewModel
import org.fictionally.ringpromoter.ui.detail.ActionRequest
import org.fictionally.ringpromoter.ui.detail.ActionSheetContent
import org.fictionally.ringpromoter.ui.detail.AppDetailScreen
import org.fictionally.ringpromoter.ui.detail.AppDetailViewModel
import org.fictionally.ringpromoter.ui.detail.TestKitScreen
import org.fictionally.ringpromoter.ui.detail.TestKitViewModel
import org.fictionally.ringpromoter.ui.groups.GroupsScreen
import org.fictionally.ringpromoter.ui.groups.GroupsViewModel
import org.fictionally.ringpromoter.ui.history.HistoryScreen
import org.fictionally.ringpromoter.ui.history.HistoryViewModel
import org.fictionally.ringpromoter.ui.job.JobScreen
import org.fictionally.ringpromoter.ui.job.JobViewModel
import org.fictionally.ringpromoter.ui.lock.LockScreen
import org.fictionally.ringpromoter.ui.maintenance.MaintenanceScreen
import org.fictionally.ringpromoter.ui.maintenance.MaintenanceViewModel
import org.fictionally.ringpromoter.ui.onboarding.OnboardingScreen
import org.fictionally.ringpromoter.ui.overview.OverviewScreen
import org.fictionally.ringpromoter.ui.overview.OverviewViewModel
import org.fictionally.ringpromoter.ui.settings.ControlPlanesScreen
import org.fictionally.ringpromoter.ui.settings.InstanceSwitcher
import org.fictionally.ringpromoter.ui.settings.ReauthenticationDialog
import org.fictionally.ringpromoter.ui.settings.SettingsScreen

/**
 * Every destination in the app.
 *
 * Path arguments are percent-encoded on the way in, so an application called
 * `team/app` cannot split its own route in two.
 */
internal object Routes {
    const val OVERVIEW = "overview"
    const val ACTIVITY = "activity"
    const val SETTINGS = "settings"
    const val CLUSTERS = "clusters"
    const val GROUPS = "groups"
    const val APP = "app/{name}"
    const val JOB = "app/{name}/job/{id}"
    const val HISTORY = "app/{name}/history"
    const val MAINTENANCE = "app/{name}/maintenance"
    const val TEST_KIT = "app/{name}/test-kit/{ring}"

    fun app(name: String) = "app/${encode(name)}"
    fun job(name: String, id: String) = "app/${encode(name)}/job/${encode(id)}"
    fun history(name: String) = "app/${encode(name)}/history"
    fun maintenance(name: String) = "app/${encode(name)}/maintenance"
    fun testKit(name: String, ring: String) = "app/${encode(name)}/test-kit/${encode(ring)}"

    private fun encode(segment: String): String = Uri.encode(segment)
}

/** The three top-level screens. */
private enum class Tab(val route: String, val label: String, val icon: ImageVector) {
    OVERVIEW(Routes.OVERVIEW, "Overview", Icons.Filled.Layers),
    ACTIVITY(Routes.ACTIVITY, "Activity", Icons.Filled.History),
    SETTINGS(Routes.SETTINGS, "Settings", Icons.Filled.Settings),
}

/**
 * Decides what the operator sees: onboarding, the lock screen, or the app.
 */
@Composable
fun RingPromoterApp(modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    val session = container.session
    val mode by session.mode.collectAsStateWithLifecycle()
    val isLocked by container.biometrics.isLocked.collectAsStateWithLifecycle()
    val lockOnOpen by session.settings.lockOnOpen.collectAsStateWithLifecycle()
    val activity = LocalActivity.current as? FragmentActivity

    // Restore the last-used control plane once, at launch.
    var restored by remember { mutableStateOf(false) }
    LaunchedEffect(Unit) {
        if (!restored) {
            session.restoreLastSession()
            restored = true
        }
    }

    when (mode) {
        is Session.Mode.Disconnected ->
            OnboardingScreen(session = session, onConnected = {}, modifier = modifier)

        else ->
            // One navigation tree per control plane: switching rebuilds every
            // screen and view model, so nothing from the previous server
            // survives on screen or in a cached state holder.
            key(session.connectionKey) {
                ConnectedApp(session = session, modifier = modifier)
            }
    }

    if (isLocked && lockOnOpen) {
        LockScreen(biometrics = container.biometrics, activity = activity)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ConnectedApp(session: Session, modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    val navController = rememberNavController()
    val backStack by navController.currentBackStackEntryAsState()
    val currentRoute = backStack?.destination?.route
    val needsReauthentication by session.needsReauthentication.collectAsStateWithLifecycle()
    val pendingLink by container.pendingLink.collectAsStateWithLifecycle()

    // A deep link from the launcher, the widget or another app. Handled here
    // rather than by navigation-compose's own matcher so the path is split
    // before it is decoded; see [DeepLink].
    LaunchedEffect(pendingLink) {
        when (val link = pendingLink) {
            null -> Unit
            is DeepLink.App -> navController.navigate(Routes.app(link.app))
            is DeepLink.History -> {
                navController.navigate(Routes.app(link.app))
                navController.navigate(Routes.history(link.app))
            }

            is DeepLink.Job -> {
                navController.navigate(Routes.app(link.app))
                navController.navigate(Routes.job(link.app, link.jobId))
            }
        }
        if (pendingLink != null) container.clearPendingLink()
    }

    val showsTabs = currentRoute in Tab.entries.map { it.route }

    Scaffold(
        modifier = modifier,
        bottomBar = {
            if (showsTabs) {
                NavigationBar {
                    Tab.entries.forEach { tab ->
                        NavigationBarItem(
                            selected = backStack?.destination?.hierarchy
                                ?.any { it.route == tab.route } == true,
                            onClick = {
                                navController.navigate(tab.route) {
                                    popUpTo(Routes.OVERVIEW) { inclusive = false }
                                    launchSingleTop = true
                                }
                            },
                            icon = { Icon(tab.icon, contentDescription = null) },
                            label = { Text(tab.label) },
                        )
                    }
                }
            }
        },
    ) { bottomInset ->
        NavHost(
            navController = navController,
            startDestination = Routes.OVERVIEW,
            modifier = Modifier.padding(bottom = bottomInset.calculateBottomPadding()),
        ) {
            composable(Routes.OVERVIEW) { OverviewRoute(session, navController) }
            composable(Routes.ACTIVITY) { ActivityRoute(session, navController) }

            composable(Routes.SETTINGS) {
                Scaffolded("Settings", session, navController, showsBack = false) { inset ->
                    SettingsScreen(
                        session = session,
                        biometrics = container.biometrics,
                        onManageClusters = { navController.navigate(Routes.CLUSTERS) },
                        onOpenGroups = { navController.navigate(Routes.GROUPS) },
                        modifier = Modifier.padding(inset),
                    )
                }
            }

            composable(Routes.CLUSTERS) {
                Scaffolded("Clusters", session, navController) { inset ->
                    ControlPlanesScreen(
                        session = session,
                        onOpened = {
                            navController.popBackStack(Routes.OVERVIEW, inclusive = false)
                        },
                        modifier = Modifier.padding(inset),
                    )
                }
            }

            composable(Routes.GROUPS) { GroupsRoute(session, navController) }
            composable(Routes.APP) { entry ->
                AppDetailRoute(session, entry.argument("name"), navController)
            }
            composable(Routes.JOB) { entry ->
                JobRoute(session, entry.argument("name"), entry.argument("id"), navController)
            }
            composable(Routes.HISTORY) { entry ->
                HistoryRoute(session, entry.argument("name"), navController)
            }
            composable(Routes.MAINTENANCE) { entry ->
                MaintenanceRoute(session, entry.argument("name"), navController)
            }
            composable(Routes.TEST_KIT) { entry ->
                TestKitRoute(
                    session,
                    entry.argument("name"),
                    entry.argument("ring"),
                    navController,
                )
            }
        }
    }

    // The token was rejected mid-session: take the operator straight to the
    // screen that can fix it rather than to an error they can dismiss.
    if (needsReauthentication && session.activeInstance != null) {
        ReauthenticationDialog(session)
    }
}

private fun androidx.navigation.NavBackStackEntry.argument(name: String): String =
    arguments?.getString(name).orEmpty()

/** The common frame: title, optional back button, and the cluster switcher. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun Scaffolded(
    title: String,
    session: Session,
    navController: NavHostController,
    showsBack: Boolean = true,
    content: @Composable (androidx.compose.foundation.layout.PaddingValues) -> Unit,
) {
    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text(title) },
                navigationIcon = {
                    if (showsBack) {
                        IconButton(onClick = { navController.popBackStack() }) {
                            Icon(Icons.Filled.ArrowBack, contentDescription = "Back")
                        }
                    }
                },
                actions = {
                    InstanceSwitcher(
                        session = session,
                        onManageClusters = { navController.navigate(Routes.CLUSTERS) },
                    )
                },
            )
        },
        content = content,
    )
}

@Composable
private fun OverviewRoute(session: Session, navController: NavHostController) {
    val vm = remember { OverviewViewModel(session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val refreshInterval by session.settings.refreshInterval.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) { vm.refresh() }
    // Periodic refresh while this screen is composed; the effect owns and
    // cancels it.
    LaunchedEffect(refreshInterval) {
        if (refreshInterval <= 0) return@LaunchedEffect
        while (true) {
            delay(refreshInterval * 1_000L)
            vm.refresh()
        }
    }

    Scaffolded("Overview", session, navController, showsBack = false) { inset ->
        OverviewScreen(
            state = state,
            onSearch = vm::setSearchText,
            onSelectGroup = vm::selectGroup,
            onOpenApp = { navController.navigate(Routes.app(it)) },
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@Composable
private fun ActivityRoute(session: Session, navController: NavHostController) {
    val vm = remember { ActivityViewModel(session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val refreshInterval by session.settings.refreshInterval.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) { vm.refresh() }
    LaunchedEffect(refreshInterval) {
        if (refreshInterval <= 0) return@LaunchedEffect
        while (true) {
            delay(refreshInterval * 1_000L)
            vm.refresh()
        }
    }

    Scaffolded("Activity", session, navController, showsBack = false) { inset ->
        ActivityScreen(
            state = state,
            title = vm::title,
            onOpenJob = { app, id -> navController.navigate(Routes.job(app, id)) },
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@Composable
private fun GroupsRoute(session: Session, navController: NavHostController) {
    val vm = remember { GroupsViewModel(session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    LaunchedEffect(Unit) { vm.refresh() }

    Scaffolded("Groups", session, navController) { inset ->
        GroupsScreen(
            state = state,
            viewModel = vm,
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppDetailRoute(
    session: Session,
    app: String,
    navController: NavHostController,
) {
    val vm = remember(app) { AppDetailViewModel(app, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    var pending by remember { mutableStateOf<ActionRequest?>(null) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    LaunchedEffect(app) { vm.refresh() }

    Scaffolded(state.title, session, navController) { inset ->
        AppDetailScreen(
            state = state,
            viewModel = vm,
            onAction = { pending = it },
            onOpenHistory = { navController.navigate(Routes.history(app)) },
            onOpenMaintenance = { navController.navigate(Routes.maintenance(app)) },
            onOpenTestKit = { navController.navigate(Routes.testKit(app, it)) },
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }

    pending?.let { request ->
        ModalBottomSheet(
            onDismissRequest = { pending = null },
            sheetState = sheetState,
        ) {
            ActionSheetContent(
                state = state,
                request = request,
                viewModel = vm,
                onStarted = { jobId ->
                    pending = null
                    navController.navigate(Routes.job(app, jobId))
                },
                onDismiss = { pending = null },
                onRecordSignoff = { _, _ -> },
                onOpenMaintenance = {
                    pending = null
                    navController.navigate(Routes.maintenance(app))
                },
            )
        }
    }
}

@Composable
private fun JobRoute(
    session: Session,
    app: String,
    jobId: String,
    navController: NavHostController,
) {
    val vm = remember(app, jobId) { JobViewModel(app, jobId, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    // Polling lives and dies with this screen.
    LaunchedEffect(app, jobId) { vm.run() }

    Scaffolded("Job", session, navController) { inset ->
        JobScreen(
            state = state,
            aiEnabled = session.aiEnabled,
            onDiagnose = { scope.launch { vm.requestDiagnosis() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@Composable
private fun HistoryRoute(session: Session, app: String, navController: NavHostController) {
    val vm = remember(app) { HistoryViewModel(app, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    LaunchedEffect(app) { vm.refresh() }

    Scaffolded("History", session, navController) { inset ->
        HistoryScreen(
            state = state,
            viewModel = vm,
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@Composable
private fun MaintenanceRoute(session: Session, app: String, navController: NavHostController) {
    val vm = remember(app) { MaintenanceViewModel(app, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    LaunchedEffect(app) { vm.refresh() }

    Scaffolded("Maintenance", session, navController) { inset ->
        MaintenanceScreen(
            state = state,
            viewModel = vm,
            onRetry = { scope.launch { vm.refresh() } },
            modifier = Modifier.padding(inset),
        )
    }
}

@Composable
private fun TestKitRoute(
    session: Session,
    app: String,
    ring: String,
    navController: NavHostController,
) {
    val vm = remember(app, ring) { TestKitViewModel(app, ring, session) }
    val state by vm.state.collectAsStateWithLifecycle()

    LaunchedEffect(app, ring) { vm.load() }

    Scaffolded("Test ${state.kit?.version.orEmpty().ifEmpty { ring }}", session, navController) {
            inset ->
        TestKitScreen(state = state, viewModel = vm, modifier = Modifier.padding(inset))
    }
}
