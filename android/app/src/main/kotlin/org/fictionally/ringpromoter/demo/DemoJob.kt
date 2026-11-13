package org.fictionally.ringpromoter.demo

import org.fictionally.ringpromoter.model.ActionResult
import org.fictionally.ringpromoter.model.DiagnosisStatus
import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.model.JobStatus
import org.fictionally.ringpromoter.model.JobStep
import org.fictionally.ringpromoter.model.PromotionAction
import org.fictionally.ringpromoter.model.RingState
import org.fictionally.ringpromoter.model.StepStatus
import java.time.Duration
import java.time.Instant

/**
 * A simulated job whose steps advance with wall-clock time.
 *
 * Deriving the snapshot from elapsed time — rather than mutating on a timer —
 * keeps demo mode free of background work and makes the progression
 * reproducible: poll at t+3s and you always get the same third step.
 */
data class DemoJob(
    val id: String,
    val app: String,
    val action: PromotionAction,
    val targetRing: String,
    val sourceRing: String?,
    val version: String,
    /** What the target ring was running before this operation. */
    val previousVersion: String,
    val startedAt: Instant,
    /**
     * When true the health check fails and the ring is rolled back, so the
     * failure path is demonstrable.
     */
    val fails: Boolean,
    /** Set once the finished job's effect has been written into the world. */
    val applied: Boolean = false,
    /** Set when someone asks for an AI diagnosis. */
    val diagnosis: String? = null,
) {
    private data class Plan(
        val id: String,
        val title: String,
        val logs: List<String>,
        val fails: Boolean,
    )

    private val plan: List<Plan>
        get() = when (action) {
            PromotionAction.PROMOTE -> buildList {
                add(
                    Plan(
                        "source-health",
                        "Verify ${sourceRing ?: "source"} ($version) is healthy",
                        listOf("source healthy"),
                        false,
                    ),
                )
                add(Plan("deploy", "Deploy $version to $targetRing", listOf("image set to $version"), false))
                add(healthStep)
                if (fails && previousVersion.isNotEmpty()) add(rollbackStep)
            }

            PromotionAction.SEED -> buildList {
                add(Plan("deploy", "Deploy $version to $targetRing", listOf("image set to $version"), false))
                add(healthStep)
                if (fails && previousVersion.isNotEmpty()) add(rollbackStep)
            }

            PromotionAction.ROLLBACK -> listOf(
                Plan("deploy", "Roll $targetRing back to $version", listOf("image set to $version"), false),
                Plan("health", "Health check $targetRing", listOf("attempt 1/3 succeeded"), false),
            )
        }

    private val healthStep: Plan
        get() = Plan(
            id = "health",
            title = "Health check $targetRing",
            logs = if (fails) {
                listOf(
                    "attempt 1/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 2/3 failed: unhealthy: status 503",
                    "waiting 5s before retry",
                    "attempt 3/3 failed: unhealthy: status 503",
                    "health check failed after retries: unhealthy: status 503",
                )
            } else {
                listOf("attempt 1/3 succeeded")
            },
            fails = fails,
        )

    private val rollbackStep: Plan
        get() = Plan(
            id = "rollback",
            title = "Auto-rollback $targetRing to $previousVersion",
            logs = listOf(
                "deploying previous version $previousVersion",
                "attempt 1/3 succeeded",
                "rolled back to $previousVersion",
            ),
            fails = false,
        )

    /** The job as it appears at [instant]. */
    fun snapshot(instant: Instant): Job {
        val plan = plan
        val elapsedMillis = Duration.between(startedAt, instant).toMillis().coerceAtLeast(0)
        val completedCount = (elapsedMillis / STEP_MILLIS).toInt().coerceAtMost(plan.size)
        val isFinished = completedCount >= plan.size

        val steps = plan.take(completedCount + 1).mapIndexed { index, item ->
            val stepStart = startedAt.plusMillis(index * STEP_MILLIS)
            val done = index < completedCount
            JobStep(
                id = item.id,
                title = item.title,
                status = when {
                    !done -> StepStatus.RUNNING
                    item.fails -> StepStatus.FAILED
                    else -> StepStatus.SUCCESS
                },
                // A running step reveals its log lines gradually.
                logs = if (done) item.logs else revealedLogs(item, instant, stepStart),
                startedAt = stepStart,
                finishedAt = if (done) stepStart.plusMillis(STEP_MILLIS) else null,
            )
        }

        return Job(
            id = id,
            app = app,
            action = action.wire,
            status = when {
                !isFinished -> JobStatus.RUNNING
                fails -> JobStatus.FAILED
                else -> JobStatus.SUCCESS
            },
            steps = steps,
            result = if (isFinished) result() else null,
            error = null,
            diagnosis = diagnosis,
            diagnosisStatus = if (diagnosis == null) DiagnosisStatus.NONE else DiagnosisStatus.DONE,
            diagnosisError = null,
            startedAt = startedAt,
            finishedAt = if (isFinished) startedAt.plusMillis(plan.size * STEP_MILLIS) else null,
        )
    }

    private fun revealedLogs(item: Plan, instant: Instant, stepStart: Instant): List<String> {
        if (item.logs.isEmpty()) return emptyList()
        val progress =
            Duration.between(stepStart, instant).toMillis().toDouble() / STEP_MILLIS
        val shown = ((progress * item.logs.size).toInt() + 1)
            .coerceIn(1, item.logs.size)
        return item.logs.take(shown)
    }

    private fun result(): ActionResult {
        val rolledBack = fails && previousVersion.isNotEmpty()
        val message = when {
            !fails -> when (action) {
                PromotionAction.SEED -> "seeded $version and healthy"
                PromotionAction.PROMOTE ->
                    "promoted $version from ${sourceRing ?: "?"} to $targetRing and healthy"

                PromotionAction.ROLLBACK -> "rolled back to $version"
            }

            rolledBack ->
                "${action.wire} of $version failed health check; rolled back to $previousVersion"

            else -> "${action.wire} of $version failed health check: unhealthy: status 503"
        }

        return ActionResult(
            app = app,
            action = action.wire,
            ring = targetRing,
            fromRing = sourceRing,
            version = version,
            success = !fails,
            rolledBack = rolledBack,
            message = message,
            state = RingState(
                app = app,
                ring = targetRing,
                currentVersion = if (fails) previousVersion else version,
                previousVersion = if (fails) version else previousVersion,
                healthy = !fails,
                autoPromote = false,
                updatedAt = startedAt.plusMillis(plan.size * STEP_MILLIS),
            ),
        )
    }

    private companion object {
        /**
         * How long each step takes. Slow enough to watch, short enough that a
         * demo does not stall.
         */
        const val STEP_MILLIS = 2_200L
    }
}
