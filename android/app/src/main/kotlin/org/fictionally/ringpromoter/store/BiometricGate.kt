package org.fictionally.ringpromoter.store

import android.content.Context
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume

/**
 * The device's own unlock, used for two different things: covering the app when
 * it leaves the foreground, and confirming an action that targets production.
 *
 * `BIOMETRIC_WEAK or DEVICE_CREDENTIAL` is deliberate. The PIN fallback means
 * an on-call engineer with a wet thumb, a cracked sensor or a face the phone
 * will not recognise at 3am can still get in. A gate that can lock out the
 * person holding the pager is worse than no gate.
 *
 * Nothing here protects data at rest — the token's own protection is the
 * Keystore key in [TokenStore], which is bound to the device being unlocked.
 * This is about shoulder-surfing and a borrowed phone.
 */
class BiometricGate(context: Context) {

    private val manager = BiometricManager.from(context.applicationContext)

    /** What the device can actually do, so Settings can say so plainly. */
    sealed interface Availability {
        /** Biometrics, or at least a device PIN, can be asked for. */
        data class Available(val label: String) : Availability

        /** Nothing can be asked for, with the reason in the operator's words. */
        data class Unavailable(val reason: String) : Availability
    }

    private val _isLocked = MutableStateFlow(false)
    val isLocked: StateFlow<Boolean> = _isLocked.asStateFlow()

    val availability: Availability
        get() = when (manager.canAuthenticate(AUTHENTICATORS)) {
            BiometricManager.BIOMETRIC_SUCCESS ->
                Availability.Available("the device unlock")

            BiometricManager.BIOMETRIC_ERROR_NO_HARDWARE,
            BiometricManager.BIOMETRIC_ERROR_HW_UNAVAILABLE,
            ->
                Availability.Unavailable("this device has no biometric hardware")

            BiometricManager.BIOMETRIC_ERROR_NONE_ENROLLED ->
                Availability.Unavailable("no fingerprint, face or screen lock is set up")

            BiometricManager.BIOMETRIC_ERROR_SECURITY_UPDATE_REQUIRED ->
                Availability.Unavailable("the device needs a security update first")

            else -> Availability.Unavailable("it is unavailable on this device")
        }

    val canAuthenticate: Boolean get() = availability is Availability.Available

    /**
     * Cover the app. Called when it leaves the foreground, so the lock is in
     * place before the task switcher can screenshot anything.
     *
     * A no-op when nothing could ever unlock it: locking a device with no PIN
     * would strand the operator behind a screen with no way past it.
     */
    fun lock() {
        if (canAuthenticate) _isLocked.value = true
    }

    /** Only the successful end of [authenticate] may call this. */
    private fun unlock() {
        _isLocked.value = false
    }

    /**
     * Ask the device to confirm who is holding it.
     *
     * Returns false for a failure *or* a cancellation; the caller treats both
     * as "not confirmed", and the prompt has already told the operator why.
     */
    suspend fun authenticate(
        activity: FragmentActivity,
        title: String,
        subtitle: String? = null,
    ): Boolean {
        if (!canAuthenticate) return false
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .apply { subtitle?.let { setSubtitle(it) } }
            // No negative button is permitted alongside DEVICE_CREDENTIAL, and
            // none is wanted: the system sheet's own dismiss is the cancel.
            .setAllowedAuthenticators(AUTHENTICATORS)
            .setConfirmationRequired(false)
            .build()

        return suspendCancellableCoroutine { cont ->
            val prompt = BiometricPrompt(
                activity,
                ContextCompat.getMainExecutor(activity),
                object : BiometricPrompt.AuthenticationCallback() {
                    override fun onAuthenticationSucceeded(
                        result: BiometricPrompt.AuthenticationResult,
                    ) {
                        unlock()
                        if (cont.isActive) cont.resume(true)
                    }

                    override fun onAuthenticationError(code: Int, message: CharSequence) {
                        if (cont.isActive) cont.resume(false)
                    }

                    // Deliberately not resumed: a single bad fingerprint is not
                    // a decision. The prompt stays up and the operator tries
                    // again, exactly as the system lock screen behaves.
                    override fun onAuthenticationFailed() = Unit
                },
            )
            prompt.authenticate(info)
            cont.invokeOnCancellation { runCatching { prompt.cancelAuthentication() } }
        }
    }

    private companion object {
        const val AUTHENTICATORS =
            BiometricManager.Authenticators.BIOMETRIC_WEAK or
                BiometricManager.Authenticators.DEVICE_CREDENTIAL
    }
}
