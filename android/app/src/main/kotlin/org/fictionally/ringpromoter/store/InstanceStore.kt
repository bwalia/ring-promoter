package org.fictionally.ringpromoter.store

import android.content.SharedPreferences
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import org.fictionally.ringpromoter.net.RpJson
import org.fictionally.ringpromoter.ui.theme.InstanceTint

/**
 * The saved control planes and which one is active.
 *
 * Records live in `SharedPreferences`; their tokens live in the [TokenStore],
 * keyed by the record's id. Deleting a record deletes its token — an orphaned
 * token is a credential nobody is managing.
 */
class InstanceStore(
    private val prefs: SharedPreferences,
    private val tokens: TokenStore,
) {
    private val _instances = MutableStateFlow<List<Instance>>(emptyList())
    val instances: StateFlow<List<Instance>> = _instances.asStateFlow()

    private val _activeId = MutableStateFlow<String?>(null)
    val activeId: StateFlow<String?> = _activeId.asStateFlow()

    init {
        load()
    }

    val active: Instance? get() = _instances.value.firstOrNull { it.id == _activeId.value }

    val isEmpty: Boolean get() = _instances.value.isEmpty()

    /**
     * Add or replace a record and store its token.
     *
     * The token is written first: if the key store refuses, no record is saved,
     * so the app never lists an instance it cannot authenticate to.
     */
    fun save(instance: Instance, token: String) {
        tokens.save(token, instance.tokenAccount)
        val current = _instances.value.toMutableList()
        val index = current.indexOfFirst { it.id == instance.id }
        if (index >= 0) current[index] = instance else current.add(instance)
        _instances.value = current
        persist()
    }

    /**
     * Update a record's metadata (name, colour, biometric preference) without
     * touching its token.
     */
    fun update(instance: Instance) {
        val current = _instances.value.toMutableList()
        val index = current.indexOfFirst { it.id == instance.id }
        if (index < 0) return
        current[index] = instance
        _instances.value = current
        persist()
    }

    fun setActive(instance: Instance) {
        _activeId.value = instance.id
        prefs.edit().putString(KEY_ACTIVE, instance.id).apply()
    }

    /** Remove a record and its token together. */
    fun remove(instance: Instance) {
        tokens.delete(instance.tokenAccount)
        _instances.value = _instances.value.filterNot { it.id == instance.id }
        if (_activeId.value == instance.id) {
            _activeId.value = null
            prefs.edit().remove(KEY_ACTIVE).apply()
        }
        persist()
    }

    /**
     * Wipe every record and every token. Offered in Settings so an operator
     * handing a device on has one action that leaves nothing behind.
     */
    fun removeAll() {
        tokens.deleteAll()
        _instances.value = emptyList()
        _activeId.value = null
        prefs.edit().remove(KEY_ACTIVE).remove(KEY_INSTANCES).apply()
    }

    fun token(for_: Instance): String? = tokens.token(for_.tokenAccount)

    /**
     * Does a record for this URL already exist? Used so adding the same server
     * twice updates its token rather than creating a confusing duplicate.
     */
    fun instance(withBaseUrl: String): Instance? =
        _instances.value.firstOrNull { it.baseUrl == withBaseUrl }

    /**
     * A colour no saved instance uses yet, so a new one is told apart from the
     * rest without the operator having to pick.
     *
     * Red is left until last: it reads as "production" and should be a
     * deliberate choice.
     */
    fun nextUnusedTint(): InstanceTint {
        val used = _instances.value.map { it.tint }.toSet()
        val order = listOf(
            InstanceTint.BLUE,
            InstanceTint.GREEN,
            InstanceTint.ORANGE,
            InstanceTint.PURPLE,
            InstanceTint.GRAPHITE,
            InstanceTint.RED,
        )
        return order.firstOrNull { it !in used } ?: InstanceTint.BLUE
    }

    private fun load() {
        prefs.getString(KEY_INSTANCES, null)?.let { raw ->
            _instances.value = runCatching {
                RpJson.decodeFromString<List<Instance>>(raw)
            }.getOrDefault(emptyList())
        }
        _activeId.value = prefs.getString(KEY_ACTIVE, null)
    }

    private fun persist() {
        val raw = runCatching {
            RpJson.encodeToString(
                kotlinx.serialization.builtins.ListSerializer(Instance.serializer()),
                _instances.value,
            )
        }.getOrNull() ?: return
        prefs.edit().putString(KEY_INSTANCES, raw).apply()
    }

    private companion object {
        const val KEY_INSTANCES = "rp.instances"
        const val KEY_ACTIVE = "rp.activeInstanceID"
    }
}
