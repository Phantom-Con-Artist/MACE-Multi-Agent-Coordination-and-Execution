package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Groups
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.RoundRect
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Outline
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.PathOperation
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.drawscope.rotate
import androidx.compose.ui.graphics.drawscope.translate
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Density
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.LayoutDirection
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.ui.theme.mace
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlin.random.Random

/** One color per agent, the same everywhere: avatars and names in groups. */
@Composable
fun memberColor(id: String): Color {
    val names = mace.names
    return names[(id.hashCode() and Int.MAX_VALUE) % names.size]
}

/** A round avatar: the agent's initial, or the group mark, on its own tint. */
@Composable
fun Avatar(name: String, id: String, group: Boolean, size: Int) {
    val color = memberColor(id)
    Box(Modifier.size(size.dp).clip(CircleShape).background(color.copy(alpha = 0.2f)), contentAlignment = Alignment.Center) {
        if (group) Icon(Icons.Rounded.Groups, null, tint = color, modifier = Modifier.size((size * 0.55).dp))
        else Text(name.trim().take(1).uppercase(), color = color, fontWeight = FontWeight.SemiBold, fontSize = (size * 0.42).sp)
    }
}

/** Time for the chat list: today's time, Yesterday, a weekday, or a date. */
fun listTime(at: String): String {
    val t = runCatching { OffsetDateTime.parse(at).atZoneSameInstant(ZoneId.systemDefault()) }.getOrNull() ?: return ""
    val day = t.toLocalDate()
    return when {
        day == LocalDate.now() -> t.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))
        day == LocalDate.now().minusDays(1) -> "Yesterday"
        day.isAfter(LocalDate.now().minusDays(7)) -> t.dayOfWeek.getDisplayName(java.time.format.TextStyle.FULL, java.util.Locale.getDefault())
        else -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.SHORT))
    }
}

/**
 * A message bubble. Every bubble keeps room for the tail on its side so a
 * run lines up; only the first of a run draws it.
 */
class BubbleShape(private val mine: Boolean, private val tail: Boolean, private val t: Dp = BubbleTail, private val r: Dp = 10.dp) : Shape {
    override fun createOutline(size: Size, layoutDirection: LayoutDirection, density: Density): Outline {
        val tp = with(density) { t.toPx() }
        val rp = with(density) { r.toPx() }
        val left = if (mine) 0f else tp
        val right = if (mine) size.width - tp else size.width
        val sharp = CornerRadius(if (tail) rp * 0.2f else rp)
        val round = CornerRadius(rp)
        val body = Path().apply {
            addRoundRect(RoundRect(left, 0f, right, size.height, if (!mine) sharp else round, if (mine) sharp else round, round, round))
        }
        if (!tail) return Outline.Generic(body)
        val tip = Path().apply {
            if (mine) { moveTo(right - 2, 0f); lineTo(size.width, 0f); lineTo(right - 2, tp * 1.5f) }
            else { moveTo(left + 2, 0f); lineTo(0f, 0f); lineTo(left + 2, tp * 1.5f) }
            close()
        }
        return Outline.Generic(Path.combine(PathOperation.Union, body, tip))
    }
}

val BubbleTail = 8.dp

/**
 * The chat background: a quiet field of MACE's geometric marks (rings,
 * triangles, squares, crosses, and the folded path of the logo).
 */
fun Modifier.chatPattern(color: Color): Modifier = drawWithCache {
    val cell = 64.dp.toPx()
    val unit = 9.dp.toPx()
    val stroke = Stroke(width = 1.3.dp.toPx())
    val rng = Random(7)
    val w = size.width
    val h = size.height
    data class Mark(val x: Float, val y: Float, val kind: Int, val turn: Float)
    val marks = buildList {
        var y = 0f
        var row = 0
        while (y < h + cell) {
            var x = if (row % 2 == 0) 0f else cell / 2
            while (x < w + cell) {
                add(Mark(x + rng.nextFloat() * cell * 0.3f, y + rng.nextFloat() * cell * 0.3f, rng.nextInt(5), rng.nextFloat() * 360f))
                x += cell
            }
            y += cell * 0.86f
            row++
        }
    }
    val triangle = Path().apply { moveTo(0f, -unit); lineTo(unit * 0.87f, unit * 0.5f); lineTo(-unit * 0.87f, unit * 0.5f); close() }
    val fold = Path().apply { moveTo(-unit, unit * 0.6f); lineTo(-unit * 0.35f, -unit * 0.6f); lineTo(unit * 0.35f, unit * 0.6f); lineTo(unit, -unit * 0.6f) }
    val cross = Path().apply { moveTo(-unit * 0.7f, 0f); lineTo(unit * 0.7f, 0f); moveTo(0f, -unit * 0.7f); lineTo(0f, unit * 0.7f) }
    val square = Path().apply { addRoundRect(RoundRect(-unit * 0.7f, -unit * 0.7f, unit * 0.7f, unit * 0.7f, CornerRadius(unit * 0.2f))) }
    onDrawBehind {
        marks.forEach { m ->
            translate(m.x, m.y) {
                rotate(m.turn, pivot = androidx.compose.ui.geometry.Offset.Zero) {
                    when (m.kind) {
                        0 -> drawCircle(color, unit * 0.75f, androidx.compose.ui.geometry.Offset.Zero, style = stroke)
                        1 -> drawPath(triangle, color, style = stroke)
                        2 -> drawPath(square, color, style = stroke)
                        3 -> drawPath(cross, color, style = stroke)
                        else -> drawPath(fold, color, style = stroke)
                    }
                }
            }
        }
    }
}

/** Asks before something that cannot be undone from the phone. */
@Composable
fun Confirm(title: String, text: String, action: String, onConfirm: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(text) },
        confirmButton = { TextButton({ onDismiss(); onConfirm() }) { Text(action, color = MaterialTheme.colorScheme.error) } },
        dismissButton = { TextButton(onDismiss) { Text("Cancel") } },
    )
}

/** A small heading above a list section, as in a messenger's settings. */
@Composable
fun SectionLabel(text: String) {
    Text(text, style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.fillMaxWidth().padding(start = 20.dp, end = 20.dp, top = 20.dp, bottom = 8.dp))
}
