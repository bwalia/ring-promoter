package org.fictionally.ringpromoter.ui.rings

import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import org.fictionally.ringpromoter.ui.theme.RpPalette
import org.fictionally.ringpromoter.ui.theme.rpPalette

/**
 * One row per app, one column per ring (int → prod), a small Earth at the end.
 *
 * The fallback when the orbit will not fit — a narrow screen or a large fleet —
 * and also a mode an operator can simply prefer. It carries exactly the same
 * information: which version is where, which is the newest, and where a gate is
 * shut.
 */
@Composable
fun LanesView(
    apps: List<DescentApp>,
    spans: List<DescentLayout.GroupSpan>,
    ringNames: List<String>,
    selectedId: String?,
    filtering: Boolean,
    onSelect: (String) -> Unit,
    onOpenGroup: (String) -> Unit,
    modifier: Modifier = Modifier,
) {
    val palette = rpPalette
    val sections = sections(apps, spans)

    LazyColumn(
        modifier = modifier.fillMaxSize().background(EarthPalette.space),
        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp),
        verticalArrangement = Arrangement.spacedBy(6.dp),
    ) {
        item {
            Row(
                Modifier.fillMaxWidth().padding(vertical = 6.dp),
                horizontalArrangement = Arrangement.spacedBy(4.dp),
            ) {
                Text(
                    "APP",
                    modifier = Modifier.width(NAME_WIDTH.dp),
                    color = Color.White.copy(alpha = 0.5f),
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace,
                    fontWeight = FontWeight.Medium,
                )
                ringNames.forEach {
                    Text(
                        it.uppercase(),
                        modifier = Modifier.weight(1f),
                        textAlign = TextAlign.Center,
                        color = Color.White.copy(alpha = 0.5f),
                        fontSize = 10.sp,
                        fontFamily = FontFamily.Monospace,
                        fontWeight = FontWeight.Medium,
                    )
                }
                Spacer(Modifier.width(12.dp))
            }
        }

        sections.forEach { section ->
            section.name?.let { name ->
                item(key = "header-${section.id}") {
                    GroupHeader(
                        name = name,
                        colour = section.colourIndex
                            ?.let { groupColour(it, palette) }
                            ?: palette.disabled,
                        clickable = section.colourIndex != null,
                        filtering = filtering,
                        onClick = { onOpenGroup(section.id) },
                    )
                }
            }
            items(section.apps, key = { it.id }) { app ->
                LaneRow(
                    app = app,
                    selected = selectedId == app.id,
                    dimmed = selectedId != null && selectedId != app.id,
                    palette = palette,
                    onTap = { onSelect(app.id) },
                )
            }
        }
    }
}

private const val NAME_WIDTH = 108

private data class Section(
    val id: String,
    val name: String?,
    /** Null for the ungrouped tail, which has no colour and no filter. */
    val colourIndex: Int?,
    val apps: List<DescentApp>,
)

private fun sections(
    apps: List<DescentApp>,
    spans: List<DescentLayout.GroupSpan>,
): List<Section> {
    val out = mutableListOf<Section>()
    var next = 0
    spans.forEachIndexed { i, span ->
        out += Section(span.id, span.name, i, apps.subList(span.from, span.to + 1))
        next = span.to + 1
    }
    if (next < apps.size) {
        out += Section(
            id = "__ungrouped__",
            name = if (spans.isEmpty()) null else "Ungrouped",
            colourIndex = null,
            apps = apps.subList(next, apps.size),
        )
    }
    return out
}

@Composable
private fun GroupHeader(
    name: String,
    colour: Color,
    clickable: Boolean,
    filtering: Boolean,
    onClick: () -> Unit,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .then(if (clickable) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(top = 6.dp, bottom = 2.dp)
            .semantics {
                contentDescription = if (filtering) {
                    "$name. Shows all apps."
                } else {
                    "Group $name. Shows only its apps."
                }
            },
        horizontalArrangement = Arrangement.spacedBy(6.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.width(14.dp).height(3.dp).background(colour, CircleShape))
        Text(
            name,
            color = Color.White.copy(alpha = 0.75f),
            fontSize = 12.sp,
            fontWeight = FontWeight.SemiBold,
        )
    }
}

@Composable
private fun LaneRow(
    app: DescentApp,
    selected: Boolean,
    dimmed: Boolean,
    palette: RpPalette,
    onTap: () -> Unit,
) {
    val alpha = if (dimmed) 0.22f else 1f
    Row(
        Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .background(Color.White.copy(alpha = (if (selected) 0.08f else 0.025f) * alpha))
            .clickable(onClick = onTap)
            .padding(horizontal = 6.dp, vertical = 5.dp)
            .semantics(mergeDescendants = true) {
                contentDescription = "${app.title}: ${app.status.label}. ${app.sentence}"
            },
        horizontalArrangement = Arrangement.spacedBy(4.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Row(
            Modifier.width(NAME_WIDTH.dp),
            horizontalArrangement = Arrangement.spacedBy(5.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier
                    .size(6.dp)
                    .background(app.status.tint(palette).copy(alpha = alpha), CircleShape),
            )
            Text(
                app.title,
                color = Color.White.copy(alpha = 0.92f * alpha),
                fontSize = 13.sp,
                fontWeight = FontWeight.Medium,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis,
            )
        }
        app.spoke.nodes.forEachIndexed { i, node ->
            VersionPill(
                node = node,
                gated = i == app.spoke.gateAt,
                alpha = alpha,
                palette = palette,
                modifier = Modifier.weight(1f),
            )
        }
        EarthDot()
    }
}

/**
 * One ring's cell: solid tint for the newest version, an outline for an older
 * one, a dashed dash for nothing deployed — fainter still when the app does not
 * use that ring at all.
 */
@Composable
private fun VersionPill(
    node: DescentLayout.SpokeNode,
    gated: Boolean,
    alpha: Float,
    palette: RpPalette,
    modifier: Modifier = Modifier,
) {
    val shape = RoundedCornerShape(6.dp)
    Box(modifier.height(24.dp), contentAlignment = Alignment.Center) {
        if (node.version != null) {
            val tint = DescentApp.tint(node.state, palette).copy(alpha = alpha)
            Box(
                Modifier
                    .fillMaxSize()
                    .background(
                        if (node.fresh) tint.copy(alpha = 0.15f * alpha) else Color.Transparent,
                        shape,
                    )
                    .border(1.dp, tint.copy(alpha = (if (node.fresh) 0.35f else 0.6f) * alpha), shape),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    node.version,
                    color = tint,
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(horizontal = 3.dp),
                )
            }
        } else {
            val off = node.state == DescentLayout.NodeState.OFF
            Box(
                Modifier
                    .fillMaxSize()
                    .border(
                        1.dp,
                        Color.White.copy(alpha = (if (off) 0.06f else 0.15f) * alpha),
                        shape,
                    ),
                contentAlignment = Alignment.Center,
            ) {
                Text(
                    "—",
                    color = Color.White.copy(alpha = (if (off) 0.2f else 0.4f) * alpha),
                    fontSize = 10.sp,
                    fontFamily = FontFamily.Monospace,
                )
            }
        }
        if (gated) {
            Icon(
                Icons.Filled.Lock,
                contentDescription = "Gate closed",
                tint = palette.gate.copy(alpha = alpha),
                modifier = Modifier.align(Alignment.TopEnd).size(8.dp),
            )
        }
    }
}

/** Earth — live users — at the end of each lane. */
@Composable
private fun EarthDot() {
    Box(
        Modifier
            .size(12.dp)
            .background(
                Brush.radialGradient(
                    listOf(EarthPalette.oceanNear, EarthPalette.oceanFar),
                    center = androidx.compose.ui.geometry.Offset(4f, 4f),
                    radius = 12f,
                ),
                CircleShape,
            )
            .border(1.dp, EarthPalette.limb.copy(alpha = 0.35f), CircleShape),
    )
}
