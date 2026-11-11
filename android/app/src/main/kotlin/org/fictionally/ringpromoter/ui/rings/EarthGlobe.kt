package org.fictionally.ringpromoter.ui.rings

import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.clipPath
import kotlin.math.PI
import kotlin.math.sin

/**
 * Earth — live users — at the centre of the stage, plus the quiet star field
 * behind it.
 *
 * Orthographic ocean, graticule, coarse land, a camera-fixed terminator and a
 * specular highlight. Continents rotating under fixed lighting is what reads as
 * a sphere rather than a flat disc; spin is the only input that moves.
 */
object EarthPalette {
    val space = Color(0.027f, 0.027f, 0.039f)
    val oceanNear = Color(28 / 255f, 74 / 255f, 110 / 255f)
    val oceanMid = Color(13 / 255f, 42 / 255f, 68 / 255f)
    val oceanFar = Color(7 / 255f, 20 / 255f, 31 / 255f)
    val night = Color(2 / 255f, 6 / 255f, 12 / 255f)
    val atmosphere = Color(56 / 255f, 189 / 255f, 248 / 255f)
    val graticule = Color(186 / 255f, 230 / 255f, 253 / 255f)
    val limb = Color(125 / 255f, 211 / 255f, 252 / 255f)
    val landFill = Color(134 / 255f, 168 / 255f, 128 / 255f)
    val landStroke = Color(190 / 255f, 210 / 255f, 170 / 255f)
    val halo = Color(245 / 255f, 185 / 255f, 66 / 255f)
}

/** Draw the globe centred on [center] with disc radius [radius] (pixels). */
fun DrawScope.drawEarth(center: Offset, radius: Float, spin: Double) {
    // Hairlines were designed at a 58dp globe; scale them with it.
    val s = maxOf(0.6f, radius / EarthGeometry.REFERENCE_RADIUS.toFloat())

    // Atmosphere.
    drawCircle(
        brush = Brush.radialGradient(
            0.0f to EarthPalette.atmosphere.copy(alpha = 0f),
            0.72f to EarthPalette.atmosphere.copy(alpha = 0.07f),
            1.0f to Color.Transparent,
            center = center,
            radius = radius * 1.28f,
        ),
        radius = radius * 1.28f,
        center = center,
    )

    val disc = Path().apply {
        addOval(
            androidx.compose.ui.geometry.Rect(
                center.x - radius,
                center.y - radius,
                center.x + radius,
                center.y + radius,
            ),
        )
    }

    clipPath(disc) {
        // Ocean. The light is camera-fixed, which is what makes the land read
        // as moving under it.
        drawCircle(
            brush = Brush.radialGradient(
                0.0f to EarthPalette.oceanNear,
                0.45f to EarthPalette.oceanMid,
                1.0f to EarthPalette.oceanFar,
                center = Offset(center.x - radius * 0.28f, center.y - radius * 0.34f),
                radius = radius,
            ),
            radius = radius,
            center = center,
        )

        val grid = Path()
        var lng = -180.0
        while (lng < 180.0) {
            var started = false
            var lat = -90.0
            while (lat <= 90.0) {
                val p = EarthGeometry.projectOrtho(
                    lat, lng, radius.toDouble(), spin,
                    center.x.toDouble(), center.y.toDouble(),
                )
                if (p.front) {
                    if (started) {
                        grid.lineTo(p.x.toFloat(), p.y.toFloat())
                    } else {
                        grid.moveTo(p.x.toFloat(), p.y.toFloat())
                        started = true
                    }
                } else {
                    started = false
                }
                lat += 4.0
            }
            lng += 30.0
        }
        var lat = -60.0
        while (lat <= 60.0) {
            var started = false
            var longitude = -180.0
            while (longitude <= 180.0) {
                val p = EarthGeometry.projectOrtho(
                    lat, longitude, radius.toDouble(), spin,
                    center.x.toDouble(), center.y.toDouble(),
                )
                if (p.front) {
                    if (started) {
                        grid.lineTo(p.x.toFloat(), p.y.toFloat())
                    } else {
                        grid.moveTo(p.x.toFloat(), p.y.toFloat())
                        started = true
                    }
                } else {
                    started = false
                }
                longitude += 5.0
            }
            lat += 30.0
        }
        drawPath(
            grid,
            color = EarthPalette.graticule.copy(alpha = 0.11f),
            style = Stroke(width = 0.55f * s),
        )

        EarthGeometry.LAND.forEach { poly ->
            val path = Path()
            var started = false
            var frontCount = 0
            poly.forEach { (lngDeg, latDeg) ->
                val p = EarthGeometry.projectOrtho(
                    latDeg, lngDeg, radius.toDouble(), spin,
                    center.x.toDouble(), center.y.toDouble(),
                )
                if (!p.front) {
                    started = false
                    return@forEach
                }
                frontCount += 1
                if (started) {
                    path.lineTo(p.x.toFloat(), p.y.toFloat())
                } else {
                    path.moveTo(p.x.toFloat(), p.y.toFloat())
                    started = true
                }
            }
            if (frontCount < 3) return@forEach
            path.close()
            drawPath(path, color = EarthPalette.landFill.copy(alpha = 0.78f))
            drawPath(
                path,
                color = EarthPalette.landStroke.copy(alpha = 0.18f),
                style = Stroke(width = 0.4f * s),
            )
        }

        // Terminator — the cue that this is a globe, not a disc.
        drawCircle(
            brush = Brush.linearGradient(
                0.00f to EarthPalette.night.copy(alpha = 0.22f),
                0.42f to EarthPalette.night.copy(alpha = 0f),
                0.62f to EarthPalette.night.copy(alpha = 0f),
                1.00f to EarthPalette.night.copy(alpha = 0.55f),
                start = Offset(center.x - radius, center.y),
                end = Offset(center.x + radius, center.y),
            ),
            radius = radius,
            center = center,
        )

        // Specular highlight on the ocean.
        val spec = Offset(center.x - radius * 0.32f, center.y - radius * 0.4f)
        drawCircle(
            brush = Brush.radialGradient(
                0.00f to Color.White.copy(alpha = 0.22f),
                0.35f to EarthPalette.graticule.copy(alpha = 0.06f),
                1.00f to Color.Transparent,
                center = spec,
                radius = radius * 0.55f,
            ),
            radius = radius * 0.55f,
            center = spec,
        )
    }

    drawCircle(
        color = EarthPalette.limb.copy(alpha = 0.28f),
        radius = radius,
        center = center,
        style = Stroke(width = 1.1f * s),
    )
    drawCircle(
        color = EarthPalette.halo.copy(alpha = 0.12f),
        radius = radius + 1.6f * s,
        center = center,
        style = Stroke(width = 0.7f * s),
    )
}

/**
 * A sparse star field, so the rings and spokes dominate rather than compete.
 *
 * The positions come from a fixed integer hash rather than a random source, so
 * the sky is the same on every frame and every launch.
 */
fun DrawScope.drawStarField(size: Size, elapsedSeconds: Double) {
    fun hash(n: Int): Double = ((n * 9301 + 49297) % 233_280).toDouble() / 233_280.0
    repeat(28) { i ->
        val x = hash(i * 3 + 1) * size.width
        val y = hash(i * 7 + 2) * size.height
        val radius = (0.8 + hash(i * 11 + 3) * 1.2) / 2
        val period = 3.5 + hash(i * 13 + 5) * 5
        val phase = hash(i * 17 + 7)
        val twinkle = 0.55 + 0.45 * sin(2 * PI * (elapsedSeconds / period + phase))
        drawCircle(
            color = Color.White.copy(alpha = (0.04 + 0.18 * twinkle).toFloat()),
            radius = radius.toFloat(),
            center = Offset(x.toFloat(), y.toFloat()),
        )
    }
}
