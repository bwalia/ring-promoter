package org.fictionally.ringpromoter.ui.groups

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
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.toMutableStateList
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.model.AppGroup
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.ui.components.EmptyState
import org.fictionally.ringpromoter.ui.components.ErrorRow

/**
 * Groups, as they are stored on the control plane.
 */
@Composable
fun GroupsScreen(
    state: GroupsState,
    viewModel: GroupsViewModel,
    onRetry: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val scope = rememberCoroutineScope()
    var editing by remember { mutableStateOf<EditRequest?>(null) }
    var pendingDeletion by remember { mutableStateOf<AppGroup?>(null) }

    LazyColumn(
        modifier = modifier.fillMaxSize(),
        contentPadding = PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(4.dp),
    ) {
        state.error?.let { item { ErrorRow(error = it, onRetry = onRetry) } }

        item {
            OutlinedButton(
                onClick = { editing = EditRequest(null) },
                modifier = Modifier.fillMaxWidth().padding(bottom = 8.dp),
            ) {
                Icon(Icons.Filled.Add, contentDescription = null, modifier = Modifier.size(16.dp))
                Spacer(Modifier.width(6.dp))
                Text("New group")
            }
        }

        items(state.groups, key = { it.id }) { group ->
            Row(
                Modifier
                    .fillMaxWidth()
                    .clickable { editing = EditRequest(group) }
                    .padding(vertical = 8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Column(Modifier.weight(1f)) {
                    Text(group.name, style = MaterialTheme.typography.titleSmall)
                    Text(
                        viewModel.memberSummary(group),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                IconButton(onClick = { pendingDeletion = group }) {
                    Icon(
                        Icons.Filled.DeleteOutline,
                        contentDescription = "Delete ${group.name}",
                    )
                }
            }
            HorizontalDivider()
        }

        if (state.isEmpty) {
            item {
                EmptyState(
                    title = "No groups yet",
                    detail = "Group related applications so the Overview can be filtered " +
                        "down to the ones you own.",
                )
            }
        }
    }

    editing?.let { request ->
        EditGroupDialog(
            group = request.group,
            viewModel = viewModel,
            onDismiss = { editing = null },
        )
    }

    pendingDeletion?.let { group ->
        AlertDialog(
            onDismissRequest = { pendingDeletion = null },
            title = { Text("Delete ${group.name}?") },
            text = {
                Text("Groups are shared, so this removes it for everyone using this server.")
            },
            confirmButton = {
                TextButton(onClick = {
                    val target = group
                    pendingDeletion = null
                    scope.launch { viewModel.delete(target) }
                }) { Text("Delete") }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeletion = null }) { Text("Keep it") }
            },
        )
    }
}

/** Nil [group] means "create a new one". */
private data class EditRequest(val group: AppGroup?)

@Composable
private fun EditGroupDialog(
    group: AppGroup?,
    viewModel: GroupsViewModel,
    onDismiss: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    var name by remember { mutableStateOf(group?.name.orEmpty()) }
    val selected = remember { group?.apps.orEmpty().toMutableStateList() }
    var isSaving by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }

    AlertDialog(
        onDismissRequest = { if (!isSaving) onDismiss() },
        title = { Text(if (group == null) "New group" else "Edit group") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it },
                    label = { Text("Group name") },
                    singleLine = true,
                    keyboardOptions = KeyboardOptions.Default,
                    modifier = Modifier.fillMaxWidth(),
                )
                Text("Applications", style = MaterialTheme.typography.labelMedium)
                LazyColumn(
                    modifier = Modifier.fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(2.dp),
                ) {
                    items(viewModel.allApps, key = { it }) { app ->
                        val isMember = app in selected
                        Row(
                            Modifier
                                .fillMaxWidth()
                                .clickable {
                                    if (isMember) selected.remove(app) else selected.add(app)
                                }
                                .padding(vertical = 8.dp)
                                .semantics(mergeDescendants = true) { },
                            verticalAlignment = Alignment.CenterVertically,
                        ) {
                            Text(
                                viewModel.title(app),
                                style = MaterialTheme.typography.bodyMedium,
                                modifier = Modifier.weight(1f),
                            )
                            if (isMember) {
                                Icon(
                                    Icons.Filled.Check,
                                    contentDescription = "Selected",
                                    modifier = Modifier.size(18.dp),
                                )
                            }
                        }
                    }
                }
                error?.let { ErrorRow(error = it) }
            }
        },
        confirmButton = {
            TextButton(
                enabled = !isSaving && name.isNotBlank(),
                onClick = {
                    scope.launch {
                        isSaving = true
                        error = null
                        try {
                            viewModel.save(group, name.trim(), selected.toSet())
                            onDismiss()
                        } catch (e: ApiError) {
                            error = e
                        } finally {
                            isSaving = false
                        }
                    }
                },
            ) {
                if (isSaving) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                } else {
                    Text("Save")
                }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, enabled = !isSaving) { Text("Cancel") }
        },
    )
}
