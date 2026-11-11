package org.fictionally.ringpromoter.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.ArrowBack
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.rememberNavController
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.LocalContainer
import org.fictionally.ringpromoter.store.Session
import org.fictionally.ringpromoter.ui.components.InstanceDot
import org.fictionally.ringpromoter.ui.detail.ActionRequest
import org.fictionally.ringpromoter.ui.detail.ActionSheetContent
import org.fictionally.ringpromoter.ui.detail.AppDetailScreen
import org.fictionally.ringpromoter.ui.detail.AppDetailViewModel
import org.fictionally.ringpromoter.ui.job.JobScreen
import org.fictionally.ringpromoter.ui.job.JobViewModel
import org.fictionally.ringpromoter.ui.onboarding.OnboardingScreen
import org.fictionally.ringpromoter.ui.overview.OverviewScreen
import org.fictionally.ringpromoter.ui.overview.OverviewViewModel
import org.fictionally.ringpromoter.ui.theme.rpPalette

private object Routes {
    const val OVERVIEW = "overview"
    const val APP = "app/{name}"
    const val JOB = "app/{name}/job/{id}"

    fun app(name: String) = "app/$name"
    fun job(name: String, id: String) = "app/$name/job/$id"
}

/**
 * Decides what the operator sees: onboarding, or the app.
 *
 * The navigation tree is keyed on the session's connection key, so switching
 * control plane rebuilds every screen and view model instead of leaving the
 * previous server's apps, rings and history on screen.
 */
@Composable
fun RingPromoterApp(modifier: Modifier = Modifier) {
    val container = LocalContainer.current
    val session = container.session
    val mode by session.mode.collectAsStateWithLifecycle()

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

        else -> ConnectedApp(session = session, modifier = modifier)
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ConnectedApp(session: Session, modifier: Modifier = Modifier) {
    val navController = rememberNavController()
    val palette = rpPalette

    NavHost(
        navController = navController,
        startDestination = Routes.OVERVIEW,
        modifier = modifier,
    ) {
        composable(Routes.OVERVIEW) {
            val vm = remember(session.connectionKey) { OverviewViewModel(session) }
            val state by vm.state.collectAsStateWithLifecycle()
            val refreshInterval by session.settings.refreshInterval.collectAsStateWithLifecycle()
            val scope = rememberCoroutineScope()

            LaunchedEffect(session.connectionKey) { vm.refresh() }
            // Periodic refresh while this screen is on the back stack's top;
            // LaunchedEffect owns and cancels it.
            LaunchedEffect(refreshInterval, session.connectionKey) {
                if (refreshInterval <= 0) return@LaunchedEffect
                while (true) {
                    delay(refreshInterval * 1_000L)
                    vm.refresh()
                }
            }

            Scaffold(
                topBar = {
                    CenterAlignedTopAppBar(
                        title = { Text("Overview") },
                        actions = { InstanceBadge(session) },
                    )
                },
            ) { inset ->
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

        composable(Routes.APP) { entry ->
            val app = entry.arguments?.getString("name").orEmpty()
            AppDetailRoute(session, app, navController)
        }

        composable(Routes.JOB) { entry ->
            val app = entry.arguments?.getString("name").orEmpty()
            val id = entry.arguments?.getString("id").orEmpty()
            JobRoute(session, app, id, navController)
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun AppDetailRoute(
    session: Session,
    app: String,
    navController: NavHostController,
) {
    val vm = remember(session.connectionKey, app) { AppDetailViewModel(app, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    var pending by remember { mutableStateOf<ActionRequest?>(null) }
    val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)

    LaunchedEffect(app, session.connectionKey) { vm.refresh() }

    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text(state.title) },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = { InstanceBadge(session) },
            )
        },
    ) { inset ->
        AppDetailScreen(
            state = state,
            viewModel = vm,
            onAction = { pending = it },
            onOpenHistory = { },
            onOpenMaintenance = { },
            onOpenTestKit = { },
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
                onOpenMaintenance = { },
            )
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun JobRoute(
    session: Session,
    app: String,
    jobId: String,
    navController: NavHostController,
) {
    val vm = remember(session.connectionKey, app, jobId) { JobViewModel(app, jobId, session) }
    val state by vm.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()

    // Polling lives and dies with this screen.
    LaunchedEffect(app, jobId) { vm.run() }

    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text("Job") },
                navigationIcon = {
                    IconButton(onClick = { navController.popBackStack() }) {
                        Icon(Icons.Filled.ArrowBack, contentDescription = "Back")
                    }
                },
                actions = { InstanceBadge(session) },
            )
        },
    ) { inset ->
        JobScreen(
            state = state,
            aiEnabled = session.aiEnabled,
            onDiagnose = { scope.launch { vm.requestDiagnosis() } },
            modifier = Modifier.padding(inset),
        )
    }
}

/**
 * Which control plane is being acted on, on every screen that can change
 * something.
 */
@Composable
private fun InstanceBadge(session: Session) {
    val palette = rpPalette
    Row(
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        modifier = Modifier.padding(end = 12.dp),
    ) {
        InstanceDot(session.instanceTint.color(palette))
        Text(session.instanceName, style = MaterialTheme.typography.labelMedium)
    }
}
