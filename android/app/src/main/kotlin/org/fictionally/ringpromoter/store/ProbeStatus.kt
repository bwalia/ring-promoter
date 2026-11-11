package org.fictionally.ringpromoter.store

import org.fictionally.ringpromoter.model.Job
import org.fictionally.ringpromoter.net.ApiError
import org.fictionally.ringpromoter.net.RingPromoterClient

/**
 * What a quick look at one saved control plane found.
 *
 * The probe is two cheap calls — `GET /api/apps` and `GET /api/jobs` — never
 * the per-app ring health checks the Overview runs. It answers "is this cluster
 * alive and is anything broken on it?" without doing the Overview's work for
 * every cluster at once.
 *
 * This lives with the stores rather than in the UI so it can be tested without
 * a device.
 */
sealed interface ProbeStatus {
    data object Checking : ProbeStatus

    /** No token in the key store — nothing can be asked of this cluster. */
    data object NoToken : ProbeStatus

    data object TokenRejected : ProbeStatus

    data class Unreachable(val message: String) : ProbeStatus

    data class Reachable(
        val apps: Int,
        val rings: Int,
        val failing: Int,
        val running: Int,
    ) : ProbeStatus

    companion object {
        /**
         * A short-timeout client per instance, so one dead host does not hold
         * up the rest of the list for a full minute.
         */
        private const val TIMEOUT_SECONDS = 8L

        suspend fun probe(instance: Instance, token: String?): ProbeStatus {
            if (token.isNullOrEmpty()) return NoToken
            val client = RingPromoterClient(
                instance.baseUrl,
                token,
                RingPromoterClient.defaultHttpClient(TIMEOUT_SECONDS),
            )
            val apps = try {
                client.apps()
            } catch (e: ApiError) {
                return if (e.requiresReauthentication) {
                    TokenRejected
                } else {
                    Unreachable(e.userMessage)
                }
            }
            // The job list is a bonus, not the point: a cluster whose /api/jobs
            // is unavailable is still reachable.
            val jobs = runCatching { client.recentJobs() }.getOrDefault(emptyList())
            val newestPerApp = jobs.groupBy { it.app }
                .values
                .mapNotNull { perApp -> perApp.maxByOrNull { it.startedAt } }
            return Reachable(
                apps = apps.apps.size,
                rings = apps.rings.size,
                failing = newestPerApp.count {
                    it.outcome == Job.Outcome.FAILED ||
                        it.outcome == Job.Outcome.FAILED_AND_ROLLED_BACK
                },
                running = newestPerApp.count { !it.isFinished },
            )
        }
    }
}
