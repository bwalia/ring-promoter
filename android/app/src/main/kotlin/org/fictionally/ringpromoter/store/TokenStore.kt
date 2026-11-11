package org.fictionally.ringpromoter.store

import android.content.Context
import android.content.SharedPreferences
import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Encrypted storage for API tokens.
 *
 * The Android equivalent of the iOS Keychain, and chosen for the same
 * properties rather than for convenience:
 *
 * - The AES key lives in the **Android Keystore**, is marked non-exportable,
 *   and never leaves the device. Combined with `android:allowBackup="false"`
 *   and the `data_extraction_rules`, a backup restored onto someone else's
 *   phone carries no usable credential — matching the iOS decision to exclude
 *   tokens from encrypted backups and device transfer.
 * - On API 28+ the key additionally requires an **unlocked device**, mirroring
 *   `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`: a background refresh cannot
 *   read a token out of a locked phone.
 *
 * The trade-off is the same one iOS documented and accepted: an operator
 * re-pastes a token after restoring a device, which beats a control-plane token
 * travelling inside a backup.
 *
 * Written directly against the Keystore rather than using
 * `androidx.security-crypto`, which is still an alpha and no longer the
 * recommended path. This is about fifty lines and says exactly what it does.
 *
 * Nothing here is ever logged.
 */
class TokenStore(context: Context) {

    private val prefs: SharedPreferences =
        context.getSharedPreferences("rp.tokens", Context.MODE_PRIVATE)

    /** Every failure the caller can act on. */
    class Failure(message: String, cause: Throwable? = null) : Exception(message, cause)

    fun save(token: String, forAccount: String) {
        try {
            val cipher = Cipher.getInstance(TRANSFORMATION).apply {
                init(Cipher.ENCRYPT_MODE, secretKey())
            }
            val ciphertext = cipher.doFinal(token.toByteArray(Charsets.UTF_8))
            // The GCM IV is generated per encryption and is not secret; it is
            // stored alongside the ciphertext, which is the standard shape.
            val packed = cipher.iv + ciphertext
            prefs.edit().putString(forAccount, Base64.encodeToString(packed, Base64.NO_WRAP))
                .apply()
        } catch (e: Exception) {
            throw Failure("Could not store the credential on this device.", e)
        }
    }

    /**
     * Returns null when no token is stored — a missing token is an ordinary
     * state (a newly added instance), not an error.
     *
     * Also returns null when the stored bytes cannot be decrypted, which
     * happens for real: the Keystore key is dropped when the user adds or
     * removes a device lock. Treating that as "no token" sends the operator to
     * the paste-a-token screen, which is the only thing they can act on.
     */
    fun token(forAccount: String): String? {
        val packed = prefs.getString(forAccount, null) ?: return null
        return try {
            val bytes = Base64.decode(packed, Base64.NO_WRAP)
            if (bytes.size <= IV_BYTES) return null
            val iv = bytes.copyOfRange(0, IV_BYTES)
            val ciphertext = bytes.copyOfRange(IV_BYTES, bytes.size)
            val cipher = Cipher.getInstance(TRANSFORMATION).apply {
                init(Cipher.DECRYPT_MODE, secretKey(), GCMParameterSpec(TAG_BITS, iv))
            }
            String(cipher.doFinal(ciphertext), Charsets.UTF_8)
        } catch (_: Exception) {
            null
        }
    }

    fun delete(forAccount: String) {
        prefs.edit().remove(forAccount).apply()
    }

    /**
     * Removes every token this app stored, and the key that protects them.
     *
     * Used by "Forget everything" in Settings, so an operator handing a device
     * on has one action that leaves nothing behind.
     */
    fun deleteAll() {
        prefs.edit().clear().apply()
        runCatching { keyStore().deleteEntry(KEY_ALIAS) }
    }

    private fun keyStore(): KeyStore =
        KeyStore.getInstance(PROVIDER).apply { load(null) }

    private fun secretKey(): SecretKey {
        val existing = keyStore().getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry
        existing?.secretKey?.let { return it }

        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, PROVIDER)
        val spec = KeyGenParameterSpec.Builder(
            KEY_ALIAS,
            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
        )
            .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
            .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
            .setKeySize(256)
            .apply {
                // Unreadable while the device is locked. API 28+ only; below
                // that the key is still Keystore-backed and non-exportable.
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                    setUnlockedDeviceRequired(true)
                }
            }
            .build()
        generator.init(spec)
        return generator.generateKey()
    }

    private companion object {
        const val PROVIDER = "AndroidKeyStore"
        const val KEY_ALIAS = "org.fictionally.ringpromoter.token"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_BYTES = 12
        const val TAG_BITS = 128
    }
}
