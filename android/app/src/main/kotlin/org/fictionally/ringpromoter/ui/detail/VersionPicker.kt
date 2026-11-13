package org.fictionally.ringpromoter.ui.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.FlowRow
import androidx.compose.foundation.layout.heightIn
import androidx.compose.material3.AssistChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import org.fictionally.ringpromoter.model.VersionsResponse

/**
 * The branches and tags the application's repository knows, filtered as the
 * operator types.
 *
 * Only offered when the server could actually enumerate versions. Anything not
 * in the list is still accepted as free text — the server resolves a commit SHA
 * against the repository before deploying — so this narrows the typing without
 * becoming a cage.
 */
@Composable
fun VersionPicker(
    versions: VersionsResponse,
    query: String,
    onPick: (String) -> Unit,
    modifier: Modifier = Modifier,
    limit: Int = 12,
) {
    val matches = versions.matching(query).take(limit)
    if (matches.isEmpty()) {
        Text(
            "No branch or tag matches. It will be sent as a commit reference.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = modifier,
        )
        return
    }

    Column(modifier = modifier, verticalArrangement = Arrangement.spacedBy(6.dp)) {
        FlowRow(
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp),
            modifier = Modifier.heightIn(max = 180.dp),
        ) {
            matches.forEach { version ->
                AssistChip(
                    onClick = { onPick(version.name) },
                    label = { Text(version.name) },
                    leadingIcon = {
                        Text(
                            if (version.isTag) "tag" else "branch",
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    },
                )
            }
        }
    }
}
