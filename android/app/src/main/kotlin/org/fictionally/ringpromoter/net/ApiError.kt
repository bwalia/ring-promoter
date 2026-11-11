package org.fictionally.ringpromoter.net

import kotlinx.serialization.Serializable

/**
 * Every way a Ring Promoter request can fail, mapped to what the operator
 * should actually do about it.
 *
 * The HTTP cases mirror the backend's status contract exactly. They are kept
 * distinct — never collapsed into "something went wrong" — because each one
 * leads somewhere different in the UI: [Unauthorized] goes to the token screen,
 * [ProductionPasswordRequired] re-opens the password field, [Conflict] explains
 * a gate.
 */
sealed class ApiError(message: String) : Exception(message) {

    /** The host could not be reached, TLS failed, or the request timed out. */
    data class Transport(val failure: TransportFailure) : ApiError(failure.message)

    /**
     * The response was not valid JSON, or did not match the expected shape.
     * Carries a short description; never the raw body, which may be large.
     */
    data class Decoding(val detail: String) : ApiError(detail)

    /** A 2xx arrived with a status the client does not handle. */
    data class UnexpectedStatus(val code: Int, val serverMessage: String?) :
        ApiError(serverMessage ?: "Unexpected response (HTTP $code).")

    /**
     * 400 — bad input: empty version, promoting past the last ring, a version
     * that does not exist in the source repo, a missing or invalid CR code.
     */
    data class BadRequest(val detail: String) : ApiError(detail)

    /**
     * 401 — the bearer token is missing, wrong or expired. Send the user to the
     * token screen.
     */
    data class Unauthorized(val detail: String) : ApiError(detail)

    /** 403 — the production password is required, or was wrong. */
    data class ProductionPasswordRequired(val detail: String) : ApiError(detail)

    /** 404 — unknown app, ring, job, window or group. */
    data class NotFound(val detail: String) : ApiError(detail)

    /**
     * 409 — nothing to promote or roll back, a shut maintenance window, a
     * missing QA sign-off, or auto-promote owned by config.
     */
    data class Conflict(val detail: String) : ApiError(detail)

    /** 501 — the server has no AI diagnosis configured. */
    data class NotImplemented(val detail: String) : ApiError(detail)

    /** 5xx. */
    data class Server(val code: Int, val detail: String) :
        ApiError(detail.ifEmpty { "The server failed to handle the request (HTTP $code)." })

    /**
     * Transport-level failures worth telling apart during onboarding, where
     * "wrong URL" and "wrong token" are the two mistakes people actually make.
     */
    sealed class TransportFailure(val message: String) {
        data class CannotReachHost(val host: String) : TransportFailure(
            "Could not reach $host. Check the server URL and that you are on " +
                "the right network or VPN.",
        )

        data class TlsFailure(val detail: String) : TransportFailure(
            "The server's TLS certificate was rejected: $detail",
        )

        data object TimedOut : TransportFailure("The server did not respond in time.")
        data object Offline : TransportFailure("This device appears to be offline.")
        data object Cancelled : TransportFailure("The request was cancelled.")
        data class Other(val detail: String) : TransportFailure(detail)
    }

    /**
     * The message to show the user. Always the server's own words when it
     * supplied any — never a bare status code.
     */
    val userMessage: String
        get() = when (this) {
            is Decoding -> "The server sent a response this app could not read. $detail"
            else -> message.orEmpty()
        }

    /** A short heading to pair with [userMessage]. */
    val title: String
        get() = when (this) {
            is Transport -> "Cannot reach the server"
            is Decoding -> "Unreadable response"
            is UnexpectedStatus -> "Unexpected response"
            is BadRequest -> "That request was rejected"
            is Unauthorized -> "Sign in again"
            is ProductionPasswordRequired -> "Production password"
            is NotFound -> "Not found"
            is Conflict -> "Blocked"
            is NotImplemented -> "Not available"
            is Server -> "Server error"
        }

    /**
     * The token is bad, so the UI must send the operator back to the token
     * screen rather than showing a toast they can dismiss and forget.
     */
    val requiresReauthentication: Boolean get() = this is Unauthorized

    /** The operator can fix this by supplying the production password. */
    val requiresProductionPassword: Boolean get() = this is ProductionPasswordRequired

    /** The user cancelled or navigated away; nothing to report. */
    val isCancellation: Boolean
        get() = this is Transport && failure is TransportFailure.Cancelled

    /** A retry might plausibly succeed without the user changing anything. */
    val isRetryable: Boolean
        get() = when (this) {
            is Server -> true
            is Transport -> failure is TransportFailure.TimedOut ||
                failure is TransportFailure.Offline ||
                failure is TransportFailure.CannotReachHost

            else -> false
        }

    companion object {
        /**
         * Build the right case from a status code and the server's decoded
         * error message. This is the single place the status contract lives.
         */
        fun from(status: Int, message: String?, host: String = "the server"): ApiError {
            val text = message?.trim().orEmpty()
            val fallback = text.ifEmpty { "The server returned HTTP $status." }
            return when (status) {
                400 -> BadRequest(fallback)
                401 -> Unauthorized(text.ifEmpty { "The API token was rejected." })
                403 -> ProductionPasswordRequired(
                    text.ifEmpty { "A production password is required for this action." },
                )

                404 -> NotFound(text.ifEmpty { "Not found on $host." })
                409 -> Conflict(fallback)
                501 -> NotImplemented(
                    text.ifEmpty { "This server does not have that feature configured." },
                )

                in 500..599 -> Server(status, text)
                else -> UnexpectedStatus(status, text.ifEmpty { null })
            }
        }
    }
}

/** The server's error body: `{"error": "..."}`. */
@Serializable
internal data class ApiErrorBody(val error: String)
