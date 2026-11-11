package org.fictionally.ringpromoter.ui.detail

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Api
import androidx.compose.material.icons.filled.Android
import androidx.compose.material.icons.filled.AutoAwesome
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Dashboard
import androidx.compose.material.icons.filled.Description
import androidx.compose.material.icons.filled.Inventory2
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.NorthEast
import androidx.compose.material.icons.filled.PhoneIphone
import androidx.compose.material.icons.filled.PlayCircleOutline
import androidx.compose.material.icons.filled.RadioButtonUnchecked
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Rocket
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.TestLink
import org.fictionally.ringpromoter.ui.components.MarkdownText
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * "Test this version": every link the ring's current version has, grouped by
 * what a tester does with it, and the AI test plan.
 */
@Composable
fun TestKitScreen(
    state: TestKitState,
    viewModel: TestKitViewModel,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val scope = rememberCoroutineScope()
    val kit = state.kit

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        state.error?.let {
            item {
                Text(it, style = MaterialTheme.typography.bodyMedium, color = palette.gate)
            }
        }

        if (kit == null) {
            if (state.error == null) {
                item { CircularProgressIndicator(Modifier.padding(16.dp)) }
            }
            return@LazyColumn
        }

        if (!state.hasLinks) {
            item {
                Text(
                    "No links yet. Add `links` to this app's config, or give the ring a " +
                        "public health URL.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        linkSection("Try it", state.tryable)
        linkSection("Build outputs", state.outputs)
        linkSection("Found in deploy logs", state.fromLogs)

        if (kit.aiEnabled) {
            item {
                Text(
                    "AI test plan",
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 10.dp),
                )
            }

            val plan = kit.plan
            when {
                state.planIsRunning -> item {
                    Row(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                        Text(
                            "Reading the deploy's links and logs…",
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }

                plan != null -> {
                    if (plan.summary.isNotEmpty()) {
                        item {
                            MarkdownText(
                                plan.summary,
                                style = MaterialTheme.typography.bodyMedium,
                            )
                        }
                    }
                    items(plan.checklist.size, key = { "check-$it" }) { index ->
                        val item = plan.checklist[index]
                        val isDone = index in state.done
                        Row(
                            Modifier
                                .fillMaxWidth()
                                .clickable { viewModel.toggle(index) }
                                .padding(vertical = 6.dp)
                                .semantics(mergeDescendants = true) { },
                            horizontalArrangement = Arrangement.spacedBy(10.dp),
                            verticalAlignment = Alignment.Top,
                        ) {
                            Icon(
                                if (isDone) {
                                    Icons.Filled.CheckCircle
                                } else {
                                    Icons.Filled.RadioButtonUnchecked
                                },
                                contentDescription = if (isDone) "Done" else "Not done",
                                tint = if (isDone) {
                                    palette.healthy
                                } else {
                                    MaterialTheme.colorScheme.onSurfaceVariant
                                },
                                modifier = Modifier.size(18.dp),
                            )
                            Text(
                                item,
                                style = MaterialTheme.typography.bodyMedium,
                                textDecoration = if (isDone) TextDecoration.LineThrough else null,
                                color = if (isDone) {
                                    MaterialTheme.colorScheme.onSurfaceVariant
                                } else {
                                    MaterialTheme.colorScheme.onSurface
                                },
                            )
                        }
                    }
                    items(plan.links, key = { "plan-${it.url}" }) { LinkRow(it) }
                    item {
                        OutlinedButton(onClick = {
                            scope.launch { viewModel.requestPlan(refresh = true) }
                        }) {
                            Icon(
                                Icons.Filled.Refresh,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                            )
                            Spacer(Modifier.width(6.dp))
                            Text("Regenerate")
                        }
                    }
                }

                else -> {
                    if (kit.planStatus == DiagnosisStatus.FAILED) {
                        kit.planError?.let {
                            item {
                                Text(
                                    it,
                                    style = MaterialTheme.typography.bodyMedium,
                                    color = palette.gate,
                                )
                            }
                        }
                    }
                    item {
                        OutlinedButton(onClick = {
                            scope.launch { viewModel.requestPlan(refresh = false) }
                        }) {
                            Icon(
                                Icons.Filled.AutoAwesome,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                            )
                            Spacer(Modifier.width(6.dp))
                            Text(
                                if (kit.planStatus == DiagnosisStatus.FAILED) {
                                    "Try again"
                                } else {
                                    "Suggest what to test"
                                },
                            )
                        }
                    }
                }
            }
        }
    }
}

private fun androidx.compose.foundation.lazy.LazyListScope.linkSection(
    title: String,
    links: List<TestLink>,
) {
    if (links.isEmpty()) return
    item(key = "header-$title") {
        Text(
            title,
            style = MaterialTheme.typography.labelLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 10.dp),
        )
    }
    items(links, key = { "$title-${it.url}" }) { LinkRow(it) }
}

@Composable
private fun LinkRow(link: TestLink) {
    val context = LocalContext.current
    val destination = link.destination ?: return

    Row(
        Modifier
            .fillMaxWidth()
            .clickable {
                // `destination` is non-null only for http(s); anything else was
                // filtered out before it reached here.
                runCatching {
                    context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(destination)))
                }
            }
            .padding(vertical = 8.dp)
            .semantics(mergeDescendants = true) { },
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(
            iconFor(link.kind),
            contentDescription = null,
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(20.dp),
        )
        Column(Modifier.weight(1f)) {
            Text(link.label, style = MaterialTheme.typography.bodyMedium)
            Text(
                link.why ?: link.shortUrl,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        Icon(
            Icons.Filled.NorthEast,
            contentDescription = "Opens in a browser",
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.size(14.dp),
        )
    }
}

private fun iconFor(kind: String): ImageVector = when (kind) {
    "web" -> Icons.Filled.Language
    "ios" -> Icons.Filled.PhoneIphone
    "android" -> Icons.Filled.Android
    "api" -> Icons.Filled.Api
    "docs" -> Icons.Filled.Description
    "dashboard" -> Icons.Filled.Dashboard
    "release" -> Icons.Filled.Rocket
    "artifact" -> Icons.Filled.Inventory2
    "ci" -> Icons.Filled.PlayCircleOutline
    else -> Icons.Filled.Language
}
