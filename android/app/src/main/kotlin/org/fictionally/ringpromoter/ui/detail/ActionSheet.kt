package org.fictionally.ringpromoter.ui.detail

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import kotlinx.coroutines.launch
import org.fictionally.ringpromoter.model.PromotionAction
import org.fictionally.ringpromoter.model.PromotionLegality
import org.fictionally.ringpromoter.model.RingStatus
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.ui.components.ErrorRow
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * What the operator asked to do.
 *
 * Carried into the sheet so one screen handles every action with the same
 * confirmation rules.
 */
sealed interface ActionRequest {
    val ring: RingStatus

    data class Promote(override val ring: RingStatus) : ActionRequest
    data class Seed(override val ring: RingStatus) : ActionRequest
    data class Rollback(override val ring: RingStatus) : ActionRequest
    data class AutoPromote(override val ring: RingStatus, val enabled: Boolean) : ActionRequest

    val action: PromotionAction
        get() = when (this) {
            is Promote, is AutoPromote -> PromotionAction.PROMOTE
            is Seed -> PromotionAction.SEED
            is Rollback -> PromotionAction.ROLLBACK
        }
}

/**
 * Collects whatever the action needs, confirms it, and starts it.
 *
 * The submit button stays disabled until every requirement is met, so the app
 * never sends a request it already knows the server will refuse. What the
 * requirements *are* comes from `PromotionRules`, not from this composable.
 */
@Composable
fun ActionSheetContent(
    state: AppDetailState,
    request: ActionRequest,
    viewModel: AppDetailViewModel,
    onStarted: (jobId: String) -> Unit,
    onDismiss: () -> Unit,
    onRecordSignoff: (ring: String, version: String) -> Unit,
    onOpenMaintenance: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val scope = rememberCoroutineScope()

    var crCode by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var seedVersion by remember { mutableStateOf("") }
    var typedAppName by remember { mutableStateOf("") }
    var isSubmitting by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<ApiError?>(null) }

    val legality: PromotionLegality = when (request) {
        is ActionRequest.Promote -> viewModel.rules.promote(request.ring, state.rings)
        is ActionRequest.Seed -> viewModel.rules.seed(request.ring)
        is ActionRequest.Rollback -> viewModel.rules.rollback(request.ring)
        is ActionRequest.AutoPromote ->
            viewModel.rules.toggleAutoPromote(request.ring, request.enabled)
    }
    val requirements = legality.requirements
    val targetRing = legality.targetRing?.let { state.ring(it) }

    /** The version this action would put into the target ring. */
    val versionInFlight = when (request) {
        is ActionRequest.Promote -> request.ring.currentVersion
        is ActionRequest.Seed -> seedVersion.trim()
        is ActionRequest.Rollback -> request.ring.previousVersion
        is ActionRequest.AutoPromote -> ""
    }

    val targetsProduction = legality.targetRing?.let { viewModel.pipeline.isProduction(it) } == true

    /**
     * Typing the app's name is the deliberate, un-fat-fingerable confirmation
     * for a production deploy. Rollback is exempt on purpose.
     */
    val requiresTypedConfirmation =
        targetsProduction && request.action != PromotionAction.ROLLBACK

    val existingSignoff = if (requirements.qaSignoff && versionInFlight.isNotEmpty()) {
        legality.targetRing?.let { state.signoffFor(it, versionInFlight) }
    } else {
        null
    }
    val signoffSatisfied = !requirements.qaSignoff || existingSignoff?.isGo == true
    val windowSatisfied = !requirements.maintenanceWindow || requirements.maintenanceWindowOpen

    val canSubmit = legality.isAllowed && !isSubmitting &&
        (!requirements.changeRequestCode || crCode.isNotBlank()) &&
        (!requirements.productionPassword || password.isNotEmpty()) &&
        (request !is ActionRequest.Seed || versionInFlight.isNotEmpty()) &&
        (!requiresTypedConfirmation || typedAppName.trim() == state.app)

    Column(
        modifier = modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp)
            .padding(bottom = 28.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Text(
            when (request) {
                is ActionRequest.Promote ->
                    "Promote ${request.ring.ring.label} → ${targetRing?.ring?.label ?: "?"}"

                is ActionRequest.Seed -> "Seed ${request.ring.ring.label}"
                is ActionRequest.Rollback -> "Roll back ${request.ring.ring.label}"
                is ActionRequest.AutoPromote ->
                    if (request.enabled) "Enable auto-promote" else "Disable auto-promote"
            },
            style = MaterialTheme.typography.titleLarge,
        )

        // The blocked case: say why, and offer nothing that would fail.
        val blocker = legality.blocker
        if (blocker != null) {
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.Top,
            ) {
                Icon(
                    Icons.Filled.Warning,
                    contentDescription = null,
                    tint = palette.unhealthy,
                    modifier = Modifier.size(18.dp),
                )
                Text(blocker.explanation, style = MaterialTheme.typography.bodyMedium)
            }
            TextButton(onClick = onDismiss, modifier = Modifier.align(Alignment.End)) {
                Text("Close")
            }
            return@Column
        }

        if (versionInFlight.isNotEmpty() && request !is ActionRequest.Seed) {
            Text(
                "Version $versionInFlight",
                style = MaterialTheme.typography.bodyMedium,
                fontFamily = FontFamily.Monospace,
            )
        }

        if (request is ActionRequest.Rollback) {
            Text(
                "Rollback is not gated: no change-request code, no maintenance window and " +
                    "no production password. Incident response is never the thing to slow down.",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        if (request is ActionRequest.Seed) {
            OutlinedTextField(
                value = seedVersion,
                onValueChange = { seedVersion = it },
                label = { Text("Version") },
                supportingText = {
                    Text(
                        if (state.versions.offersPicker) {
                            "A branch, tag or commit this application's repository knows."
                        } else {
                            "This application's deployer cannot list versions; type one."
                        },
                    )
                },
                singleLine = true,
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.None,
                    imeAction = ImeAction.Next,
                ),
                modifier = Modifier.fillMaxWidth(),
            )
            if (state.versions.offersPicker) {
                VersionPicker(
                    versions = state.versions,
                    query = seedVersion,
                    onPick = { seedVersion = it },
                )
            }
        }

        if (requirements.changeRequestCode) {
            OutlinedTextField(
                value = crCode,
                onValueChange = { crCode = it },
                label = { Text("Change-request code") },
                supportingText = {
                    Text(
                        requirements.changeRequestProvider
                            ?.let { "Validated against $it." }
                            ?: "Required to enter this ring.",
                    )
                },
                singleLine = true,
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.Characters,
                    imeAction = ImeAction.Next,
                ),
                modifier = Modifier.fillMaxWidth(),
            )
        }

        if (requirements.maintenanceWindow) {
            GateRow(
                title = "Maintenance window",
                satisfied = windowSatisfied,
                detail = if (windowSatisfied) {
                    "A window is open for ${legality.targetRing}."
                } else {
                    "No window is open for ${legality.targetRing}. " +
                        "The server would refuse this."
                },
                actionLabel = if (windowSatisfied) null else "Open a window",
                onAction = onOpenMaintenance,
            )
        }

        if (requirements.qaSignoff) {
            GateRow(
                title = "QA sign-off",
                satisfied = signoffSatisfied,
                detail = when {
                    versionInFlight.isEmpty() -> "Choose a version first."
                    existingSignoff == null ->
                        "No sign-off recorded for $versionInFlight. " +
                            "A sign-off authorises that exact version and nothing else."

                    existingSignoff.isGo ->
                        "GO by ${existingSignoff.engineer} for $versionInFlight."

                    else -> "NO-GO by ${existingSignoff.engineer} for $versionInFlight."
                },
                actionLabel = if (signoffSatisfied || versionInFlight.isEmpty()) null
                else "Record a sign-off",
                onAction = {
                    legality.targetRing?.let { onRecordSignoff(it, versionInFlight) }
                },
            )
        }

        if (requirements.productionPassword) {
            HorizontalDivider()
            Text(
                "This deploys into ${legality.targetRing}.",
                style = MaterialTheme.typography.titleSmall,
                color = palette.production,
            )
            OutlinedTextField(
                value = password,
                onValueChange = { password = it },
                label = { Text("Production password") },
                singleLine = true,
                visualTransformation = PasswordVisualTransformation(),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Next),
                modifier = Modifier.fillMaxWidth(),
            )
        }

        if (requiresTypedConfirmation) {
            OutlinedTextField(
                value = typedAppName,
                onValueChange = { typedAppName = it },
                label = { Text("Type ${state.app} to confirm") },
                supportingText = { Text("Deliberately awkward. This is production.") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(
                    capitalization = KeyboardCapitalization.None,
                    imeAction = ImeAction.Done,
                ),
                modifier = Modifier.fillMaxWidth(),
            )
        }

        error?.let { ErrorRow(error = it) }

        Row(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp, Alignment.End),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            TextButton(onClick = onDismiss, enabled = !isSubmitting) { Text("Cancel") }
            Button(
                enabled = canSubmit,
                onClick = {
                    scope.launch {
                        isSubmitting = true
                        error = null
                        try {
                            val jobId = when (request) {
                                is ActionRequest.Promote -> viewModel.promote(
                                    request.ring.ring.name,
                                    crCode.takeIf { it.isNotBlank() },
                                    password.takeIf { it.isNotEmpty() },
                                )

                                is ActionRequest.Seed -> viewModel.seed(
                                    request.ring.ring.name,
                                    versionInFlight,
                                    crCode.takeIf { it.isNotBlank() },
                                    password.takeIf { it.isNotEmpty() },
                                )

                                is ActionRequest.Rollback ->
                                    viewModel.rollback(request.ring.ring.name)

                                is ActionRequest.AutoPromote -> {
                                    viewModel.setAutoPromote(
                                        request.ring.ring.name,
                                        request.enabled,
                                        password.takeIf { it.isNotEmpty() },
                                    )
                                    null
                                }
                            }
                            if (jobId != null) onStarted(jobId) else onDismiss()
                        } catch (e: ApiError) {
                            error = e
                        } finally {
                            isSubmitting = false
                        }
                    }
                },
            ) {
                if (isSubmitting) {
                    CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
                } else {
                    Text(request.action.title)
                }
            }
        }
    }
}

/**
 * One gate, stated plainly: what it wants, whether it is satisfied, and the one
 * thing that would satisfy it.
 */
@Composable
private fun GateRow(
    title: String,
    satisfied: Boolean,
    detail: String,
    actionLabel: String?,
    onAction: () -> Unit,
) {
    val palette = rpPalette
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            horizontalArrangement = Arrangement.spacedBy(8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(
                imageVector = if (satisfied) Icons.Filled.CheckCircle else Icons.Filled.Warning,
                contentDescription = if (satisfied) "satisfied" else "not satisfied",
                tint = if (satisfied) palette.healthy else palette.gate,
                modifier = Modifier.size(16.dp),
            )
            Text(title, style = MaterialTheme.typography.titleSmall)
        }
        Text(
            detail,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (actionLabel != null) {
            OutlinedButton(onClick = onAction) { Text(actionLabel) }
        }
    }
}
