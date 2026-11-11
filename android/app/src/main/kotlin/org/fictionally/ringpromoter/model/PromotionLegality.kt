package org.fictionally.ringpromoter.model

/**
 * Everything the app must know before it may offer an action.
 *
 * The server enforces these rules; this type exists so the **UI never invites
 * an operator to do something illegal**. Every enable/disable decision in the
 * action sheets comes from here, so the rules are stated once and unit-tested
 * once rather than re-derived in each view.
 */
data class PromotionLegality(
    /** The action can be offered. */
    val isAllowed: Boolean,
    /** Why not, when it cannot. Null when allowed. */
    val blocker: Blocker?,
    /** What the sheet must collect. Meaningful only when allowed. */
    val requirements: Requirements,
    /** The ring the action deploys into. */
    val targetRing: String?,
) {
    /**
     * Why an action cannot be offered.
     *
     * Each case maps to the status the server would have returned, so the copy
     * the user reads matches what would have happened.
     */
    sealed interface Blocker {
        val explanation: String

        /** The app does not declare this ring (404). */
        data object RingNotConfigured : Blocker {
            override val explanation = "This ring is not part of the application's pipeline."
        }

        /** The source ring holds no version (409 `nothing to promote`). */
        data object SourceRingEmpty : Blocker {
            override val explanation = "There is no version in this ring to promote."
        }

        /** The source ring is unhealthy; the server refuses to promote out of it. */
        data object SourceRingUnhealthy : Blocker {
            override val explanation =
                "The source ring is unhealthy. Promotion is blocked until it recovers."
        }

        /** Already at the last ring (400 `no next ring`). */
        data object NoNextRing : Blocker {
            override val explanation =
                "This is the last ring — there is nowhere further to promote."
        }

        /** No previous version to return to (409 `nothing to roll back`). */
        data object NoPreviousVersion : Blocker {
            override val explanation = "This ring has no previous version to roll back to."
        }

        /** A maintenance window guards the target and none is open (409). */
        data class MaintenanceWindowClosed(val ring: String) : Blocker {
            override val explanation =
                "No maintenance window is open for $ring. " +
                    "Open one, or wait for a scheduled window."
        }

        /** Config owns the auto-promote switch (409). */
        data object AutoPromoteManagedByConfig : Blocker {
            override val explanation =
                "Auto-promote for this ring is declared in the server's config " +
                    "and can only be changed there."
        }
    }

    /** Requirements a sheet must collect before the request can be sent. */
    data class Requirements(
        /** The target ring needs a change-request code. */
        val changeRequestCode: Boolean = false,
        /** The CR validation backend, for the field's help text. */
        val changeRequestProvider: String? = null,
        /** The target ring needs a recorded GO for the exact version. */
        val qaSignoff: Boolean = false,
        /** The target ring is behind a maintenance-window gate. */
        val maintenanceWindow: Boolean = false,
        /** A window is open right now. */
        val maintenanceWindowOpen: Boolean = false,
        /** The action deploys into the last ring on a password-protected server. */
        val productionPassword: Boolean = false,
    ) {
        val isEmpty: Boolean
            get() = !changeRequestCode && !qaSignoff && !maintenanceWindow && !productionPassword
    }

    companion object {
        fun allowed(target: String?, requirements: Requirements = Requirements()) =
            PromotionLegality(true, null, requirements, target)

        fun blocked(blocker: Blocker, target: String? = null) =
            PromotionLegality(false, blocker, Requirements(), target)
    }
}

/**
 * The rules, evaluated against a pipeline snapshot.
 *
 * [prodProtected] comes from `GET /api/apps`; it is never assumed.
 */
class PromotionRules(
    private val pipeline: RingPipeline,
    private val prodProtected: Boolean,
) {
    /**
     * May a version be promoted out of [source], and what must be collected
     * first?
     *
     * Deliberately trusts the server's [RingStatus.canPromoteFrom] as the
     * primary gate — it is the same predicate the server applies — and only
     * then derives the specific reason for the "no" so the user gets a real
     * explanation.
     */
    fun promote(source: RingStatus, rings: List<RingStatus>): PromotionLegality {
        if (!source.configured) {
            return PromotionLegality.blocked(PromotionLegality.Blocker.RingNotConfigured)
        }
        val next = pipeline.next(source.ring.name)
            ?: return PromotionLegality.blocked(PromotionLegality.Blocker.NoNextRing)
        if (source.isEmpty) {
            return PromotionLegality.blocked(
                PromotionLegality.Blocker.SourceRingEmpty, next.name,
            )
        }
        if (!source.canPromoteFrom) {
            // The server said no while a version is present: the remaining
            // reason it enforces is source health.
            return PromotionLegality.blocked(
                if (source.isHealthy) PromotionLegality.Blocker.RingNotConfigured
                else PromotionLegality.Blocker.SourceRingUnhealthy,
                next.name,
            )
        }
        val target = rings.firstOrNull { it.ring.name == next.name }
        return PromotionLegality.allowed(
            target = next.name,
            requirements = requirements(next.name, target?.gates ?: RingGates.NONE),
        )
    }

    /**
     * May [ring] be seeded with a version?
     *
     * Seeding is legal for any configured ring; the gates guarding entry into
     * that ring still apply, and seeding the last ring needs the production
     * password just as promoting into it does.
     */
    fun seed(ring: RingStatus): PromotionLegality {
        if (!ring.configured) {
            return PromotionLegality.blocked(PromotionLegality.Blocker.RingNotConfigured)
        }
        return PromotionLegality.allowed(
            target = ring.ring.name,
            requirements = requirements(ring.ring.name, ring.gates),
        )
    }

    /**
     * May [ring] be rolled back?
     *
     * Rollback is deliberately the least gated action in the whole app:
     * incident response is never blocked, and the server exempts it from the
     * production password.
     */
    fun rollback(ring: RingStatus): PromotionLegality {
        if (!ring.configured) {
            return PromotionLegality.blocked(PromotionLegality.Blocker.RingNotConfigured)
        }
        if (!ring.canRollBack) {
            return PromotionLegality.blocked(
                PromotionLegality.Blocker.NoPreviousVersion, ring.ring.name,
            )
        }
        return PromotionLegality.allowed(target = ring.ring.name)
    }

    /**
     * May the auto-promote switch for [ring] be toggled to [enabled]?
     *
     * Enabling the hands-free path INTO production needs the production
     * password — otherwise auto-promote would be a way around it. Disabling is
     * always allowed because it only makes things safer.
     */
    fun toggleAutoPromote(ring: RingStatus, enabled: Boolean): PromotionLegality {
        if (ring.autoPromoteManaged) {
            return PromotionLegality.blocked(
                PromotionLegality.Blocker.AutoPromoteManagedByConfig, ring.ring.name,
            )
        }
        return PromotionLegality.allowed(
            target = ring.ring.name,
            requirements = PromotionLegality.Requirements(
                productionPassword = enabled && prodProtected &&
                    pipeline.promotingTargetsProduction(ring.ring.name),
            ),
        )
    }

    /** What entering [ring] demands, given its gates. */
    private fun requirements(ring: String, gates: RingGates) = PromotionLegality.Requirements(
        changeRequestCode = gates.changeRequest,
        changeRequestProvider = gates.changeRequestProvider,
        qaSignoff = gates.qaSignoff,
        maintenanceWindow = gates.maintenanceWindow,
        maintenanceWindowOpen = gates.maintenanceWindowOpen,
        productionPassword = prodProtected && pipeline.isProduction(ring),
    )
}
