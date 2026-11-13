package org.fictionally.ringpromoter.model

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import java.time.Duration
import java.time.Instant

/** Status of one step within a job. */
@Serializable(with = StepStatusSerializer::class)
enum class StepStatus(val wire: String) {
    RUNNING("running"),
    SUCCESS("success"),
    FAILED("failed"),

    /** Anything the server adds later renders neutrally rather than crashing. */
    UNKNOWN("unknown");

    val isTerminal: Boolean get() = this != RUNNING

    companion object {
        fun from(raw: String): StepStatus? = entries.firstOrNull { it.wire == raw }
    }
}

internal object StepStatusSerializer : FallbackEnumSerializer<StepStatus>(
    "StepStatus", { it.wire }, { StepStatus.from(it) }, StepStatus.UNKNOWN,
)

/** Status of a whole job. */
@Serializable(with = JobStatusSerializer::class)
enum class JobStatus(val wire: String) {
    PENDING("pending"),
    RUNNING("running"),
    SUCCESS("success"),
    FAILED("failed"),
    UNKNOWN("unknown");

    /**
     * Polling stops here. [UNKNOWN] is deliberately NOT terminal: an
     * unrecognised status is more likely a newer in-flight state than a
     * finished one, and the job's own `finishedAt` still ends the poll.
     */
    val isTerminal: Boolean get() = this == SUCCESS || this == FAILED

    companion object {
        fun from(raw: String): JobStatus? = entries.firstOrNull { it.wire == raw }
    }
}

internal object JobStatusSerializer : FallbackEnumSerializer<JobStatus>(
    "JobStatus", { it.wire }, { JobStatus.from(it) }, JobStatus.UNKNOWN,
)

/** Progress of the AI diagnosis attached to a failed job. */
@Serializable(with = DiagnosisStatusSerializer::class)
enum class DiagnosisStatus(val wire: String) {
    NONE("none"),
    RUNNING("running"),
    DONE("done"),
    FAILED("failed");

    companion object {
        fun from(raw: String): DiagnosisStatus? = entries.firstOrNull { it.wire == raw }
    }
}

internal object DiagnosisStatusSerializer : FallbackEnumSerializer<DiagnosisStatus>(
    "DiagnosisStatus", { it.wire }, { DiagnosisStatus.from(it) }, DiagnosisStatus.NONE,
)

/** One step of a running operation, with its own log lines. */
@Serializable
data class JobStep(
    val id: String,
    val title: String,
    val status: StepStatus,
    val logs: List<String> = emptyList(),
    @SerialName("started_at")
    @Serializable(with = InstantSerializer::class)
    val startedAt: Instant,
    @SerialName("finished_at")
    @Serializable(with = InstantSerializer::class)
    val finishedAt: Instant? = null,
) {
    val duration: Duration? get() = finishedAt?.let { Duration.between(startedAt, it) }
}

/**
 * A live seed / promote / rollback started with `?async=1`.
 *
 * Mirrors the API's `jobState`. Polled from `GET /api/apps/{app}/jobs/{id}`.
 */
@Serializable
data class Job(
    val id: String,
    val app: String,
    /** "seed" | "promote" | "rollback" */
    val action: String,
    val status: JobStatus,
    val steps: List<JobStep> = emptyList(),
    /** Present once the operation finished, whether or not it succeeded. */
    val result: ActionResult? = null,
    /**
     * A transport/precondition error that stopped the job before it produced a
     * result.
     */
    val error: String? = null,
    val diagnosis: String? = null,
    @SerialName("diagnosis_status") val diagnosisStatus: DiagnosisStatus = DiagnosisStatus.NONE,
    @SerialName("diagnosis_error") val diagnosisError: String? = null,
    @SerialName("started_at")
    @Serializable(with = InstantSerializer::class)
    val startedAt: Instant,
    @SerialName("finished_at")
    @Serializable(with = InstantSerializer::class)
    val finishedAt: Instant? = null,
) {
    /** The terminal state to render. */
    enum class Outcome { RUNNING, SUCCEEDED, FAILED_AND_ROLLED_BACK, FAILED }

    val promotionAction: PromotionAction? get() = PromotionAction.from(action)

    /**
     * Polling stops when the server says the job reached a terminal status OR
     * stamped a finish time. Either alone is enough.
     */
    val isFinished: Boolean get() = status.isTerminal || finishedAt != null

    /**
     * Distinguishes rolled-back from plain failure, which the UI must state
     * explicitly.
     */
    val outcome: Outcome
        get() {
            if (!isFinished) return Outcome.RUNNING
            result?.let {
                return when (it.outcome) {
                    ActionResult.Outcome.SUCCEEDED -> Outcome.SUCCEEDED
                    ActionResult.Outcome.FAILED_AND_ROLLED_BACK -> Outcome.FAILED_AND_ROLLED_BACK
                    ActionResult.Outcome.FAILED -> Outcome.FAILED
                }
            }
            return if (status == JobStatus.SUCCESS) Outcome.SUCCEEDED else Outcome.FAILED
        }

    /** The best single sentence explaining what happened. */
    val summaryMessage: String?
        get() = result?.message?.takeIf { it.isNotEmpty() }
            ?: error?.takeIf { it.isNotEmpty() }

    /** Only failed jobs can be diagnosed (the server returns 409 otherwise). */
    val canRequestDiagnosis: Boolean
        get() = isFinished && status == JobStatus.FAILED &&
            diagnosis.isNullOrEmpty() && diagnosisStatus != DiagnosisStatus.RUNNING

    /** Every log line in order, prefixed by step, for share/copy. */
    val transcript: String
        get() = steps.joinToString("\n\n") { step ->
            (listOf(step.title) + step.logs.map { "  $it" }).joinToString("\n")
        }
}

/** `POST .../seed|promote|rollback?async=1` → 202 */
@Serializable
data class JobHandle(@SerialName("job_id") val jobId: String)

/** `GET /api/jobs` — the newest job per app, shared across all operators. */
@Serializable
data class JobsResponse(val jobs: List<Job> = emptyList())

/** Response of the diagnose endpoints. */
@Serializable
data class DiagnosisResponse(
    @SerialName("diagnosis_status") val diagnosisStatus: DiagnosisStatus = DiagnosisStatus.NONE,
    val diagnosis: String? = null,
)
