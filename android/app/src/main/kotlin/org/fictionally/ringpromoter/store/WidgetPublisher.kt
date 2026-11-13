package org.fictionally.ringpromoter.store

import org.fictionally.ringpromoter.ui.overview.AppSummary

/**
 * Somewhere to publish what the home-screen widget shows.
 *
 * An interface so [Session] does not depend on Glance, and so tests can run the
 * refresh path without a widget host.
 */
interface WidgetPublisher {
    /** Called after a successful Overview refresh. Implementations rate-limit. */
    fun publish(instanceName: String, instanceTint: String, isDemo: Boolean, apps: List<AppSummary>)

    /**
     * Called on sign-out. A widget must not outlive the session it describes:
     * leaving yesterday's production pipeline on a home screen after the
     * operator signed out is a quiet way to mislead them.
     */
    fun clear()
}
