package org.fictionally.ringpromoter.ui.rings

import kotlin.math.PI
import kotlin.math.cos
import kotlin.math.floor
import kotlin.math.sin

/**
 * The maths behind the globe at the centre of the stage: orthographic
 * projection, the spin clock, and the coarse continent outlines.
 *
 * Earth stands for live users — the thing the last ring deploys to. Ported
 * from `ios/.../EarthGlobe.swift`, which matches the web console's canvas, so
 * all three draw the same planet.
 */
object EarthGeometry {
    /** Earth radius (dp) the hairline widths were designed at. */
    const val REFERENCE_RADIUS = 58.0

    /** Seconds per revolution. */
    const val SPIN_PERIOD = 96.0

    /**
     * Reduced motion: one revolution per 20 minutes.
     *
     * Not frozen outright. A globe that never moves reads as a flat disc, and
     * the point of the spin is to say "this is a sphere, and it is live". Slow
     * enough that nothing on screen appears to move.
     */
    const val SPIN_PERIOD_REDUCED = 20.0 * 60.0

    data class GlobePoint(val x: Double, val y: Double, val z: Double) {
        /** Facing the camera, so it should be drawn. */
        val front: Boolean get() = z >= -0.5
    }

    fun spin(elapsedSeconds: Double, reduceMotion: Boolean): Double {
        val period = if (reduceMotion) SPIN_PERIOD_REDUCED else SPIN_PERIOD
        val turns = elapsedSeconds / period
        return (turns - floor(turns)) * 2 * PI
    }

    fun projectOrtho(
        latDeg: Double,
        lngDeg: Double,
        radius: Double,
        spin: Double,
        centerX: Double = 0.0,
        centerY: Double = 0.0,
    ): GlobePoint {
        val lat = latDeg * PI / 180
        val lng = lngDeg * PI / 180 + spin
        val cosLat = cos(lat)
        return GlobePoint(
            x = centerX + radius * cosLat * sin(lng),
            y = centerY - radius * sin(lat),
            z = radius * cosLat * cos(lng),
        )
    }

    /** Coarse continent outlines as (lng, lat) pairs, matching the web globe. */
    val LAND: List<List<Pair<Double, Double>>> = listOf(
        listOf(
            -168.0 to 65.0, -141.0 to 70.0, -128.0 to 71.0, -105.0 to 68.0, -89.0 to 68.0,
            -80.0 to 62.0, -70.0 to 58.0, -60.0 to 47.0, -67.0 to 44.0, -74.0 to 40.0,
            -81.0 to 25.0, -97.0 to 26.0, -106.0 to 22.0, -110.0 to 24.0, -117.0 to 32.0,
            -124.0 to 40.0, -124.0 to 48.0, -130.0 to 55.0, -153.0 to 57.0, -166.0 to 54.0,
            -168.0 to 65.0,
        ),
        listOf(
            -73.0 to 76.0, -60.0 to 82.0, -20.0 to 81.0, -22.0 to 70.0, -44.0 to 60.0,
            -58.0 to 61.0, -73.0 to 76.0,
        ),
        listOf(
            -81.0 to 12.0, -60.0 to 8.0, -50.0 to 0.0, -35.0 to -8.0, -38.0 to -20.0,
            -54.0 to -35.0, -68.0 to -55.0, -75.0 to -50.0, -73.0 to -18.0, -81.0 to -5.0,
            -81.0 to 12.0,
        ),
        listOf(
            -10.0 to 52.0, -9.0 to 43.0, -1.0 to 43.0, 3.0 to 42.0, 10.0 to 44.0,
            16.0 to 40.0, 29.0 to 41.0, 30.0 to 46.0, 24.0 to 60.0, 12.0 to 58.0,
            5.0 to 61.0, -5.0 to 59.0, -10.0 to 52.0,
        ),
        listOf(
            -17.0 to 21.0, -10.0 to 12.0, 8.0 to 5.0, 10.0 to -4.0, 14.0 to -12.0,
            40.0 to -16.0, 32.0 to -28.0, 20.0 to -35.0, 18.0 to -32.0, 12.0 to -17.0,
            -5.0 to -5.0, -14.0 to 4.0, -17.0 to 14.0, -6.0 to 36.0, 10.0 to 37.0,
            25.0 to 32.0, 32.0 to 31.0, 11.0 to 33.0, -5.0 to 36.0, -17.0 to 28.0,
            -17.0 to 21.0,
        ),
        listOf(
            27.0 to 40.0, 36.0 to 36.0, 44.0 to 40.0, 60.0 to 37.0, 67.0 to 25.0,
            77.0 to 8.0, 80.0 to 15.0, 88.0 to 22.0, 73.0 to 25.0, 62.0 to 25.0,
            48.0 to 30.0, 36.0 to 21.0, 32.0 to 31.0, 27.0 to 40.0,
        ),
        listOf(
            30.0 to 60.0, 40.0 to 68.0, 70.0 to 72.0, 90.0 to 75.0, 130.0 to 71.0,
            160.0 to 66.0, 180.0 to 65.0, 170.0 to 60.0, 142.0 to 46.0, 130.0 to 43.0,
            122.0 to 30.0, 105.0 to 20.0, 100.0 to 10.0, 104.0 to 1.0, 98.0 to 8.0,
            94.0 to 18.0, 78.0 to 28.0, 74.0 to 40.0, 80.0 to 50.0, 60.0 to 50.0,
            45.0 to 55.0, 30.0 to 60.0,
        ),
        listOf(
            95.0 to 6.0, 104.0 to -6.0, 119.0 to -8.0, 131.0 to -8.0, 120.0 to 5.0,
            105.0 to 7.0, 95.0 to 6.0,
        ),
        listOf(
            113.0 to -22.0, 114.0 to -34.0, 137.0 to -35.0, 153.0 to -28.0, 153.0 to -12.0,
            142.0 to -11.0, 129.0 to -14.0, 113.0 to -22.0,
        ),
        listOf(166.0 to -41.0, 178.0 to -37.0, 178.0 to -46.0, 166.0 to -47.0, 166.0 to -41.0),
        listOf(
            -180.0 to -72.0, -90.0 to -70.0, 0.0 to -70.0, 90.0 to -72.0, 180.0 to -72.0,
            180.0 to -90.0, -180.0 to -90.0, -180.0 to -72.0,
        ),
    )
}
