package org.fictionally.ringpromoter

import android.app.Application

/**
 * Holds the one container every screen reads its dependencies from.
 *
 * No DI framework: the iOS app injects through SwiftUI environment objects, and
 * the Android equivalent is a single container reached through a
 * `CompositionLocal`. One fewer thing to understand at 3am.
 */
class RingPromoterApp : Application()
