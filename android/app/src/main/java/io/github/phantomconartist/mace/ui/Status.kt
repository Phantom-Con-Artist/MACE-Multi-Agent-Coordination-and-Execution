package io.github.phantomconartist.mace.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Reply
import androidx.compose.material.icons.rounded.Campaign
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.data.AgentStatus
import io.github.phantomconartist.mace.data.StatusOffer
import io.github.phantomconartist.mace.ui.theme.mace
import java.time.Duration
import java.time.OffsetDateTime

// The status board, laid out like a messenger's updates: open statuses with
// a ring until seen, closed ones below, and a full-screen viewer that steps
// through them. See docs/DESIGN-STATUS.md.

/** "27 minutes ago", "3 hours ago", or a time for older ones. */
fun statusAge(at: String): String {
    val t = runCatching { OffsetDateTime.parse(at) }.getOrNull() ?: return ""
    val d = Duration.between(t, OffsetDateTime.now())
    return when {
        d.toMinutes() < 1 -> "Just now"
        d.toMinutes() == 1L -> "1 minute ago"
        d.toMinutes() < 60 -> "${d.toMinutes()} minutes ago"
        d.toHours() == 1L -> "1 hour ago"
        d.toHours() < 24 -> "${d.toHours()} hours ago"
        else -> listTime(at)
    }
}

/** An avatar inside a ring: the action color while unseen, a quiet outline after. */
@Composable
private fun RingAvatar(st: AgentStatus, model: AppModel, seen: Boolean, size: Int = 52) {
    val who = model.memberForStatus(st.forId)
    val ring = if (seen || !st.open) MaterialTheme.colorScheme.outline else mace.signal
    Box(Modifier.size(size.dp), contentAlignment = Alignment.Center) {
        Canvas(Modifier.fillMaxSize()) {
            drawCircle(ring, radius = this.size.minDimension / 2 - 1.5.dp.toPx(), style = Stroke(width = (if (seen) 1.5 else 2.5).dp.toPx()))
        }
        Box(Modifier.size((size - 10).dp).clip(CircleShape)) {
            Avatar(st.forName, who?.id ?: st.forId, false, size - 10, who?.let { model.media.avatarUrl(it) }, model.media)
        }
    }
}

@Composable
fun StatusTab(statuses: List<AgentStatus>, model: AppModel, jobs: List<io.github.phantomconartist.mace.data.Job> = emptyList(), onView: (List<AgentStatus>, Int) -> Unit) {
    val open = statuses.filter { it.open }
    val closed = statuses.filter { !it.open }
    val activeJobs = jobs.filter { it.state != "approved" }
    val doneJobs = jobs.filter { it.state == "approved" }.takeLast(5)
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 24.dp)) {
        if (activeJobs.isNotEmpty() || doneJobs.isNotEmpty()) item { StatusSection("Jobs") }
        items(activeJobs.size, key = { "j" + activeJobs[it].id }) { i -> JobRow(activeJobs[i]) }
        items(doneJobs.size, key = { "d" + doneJobs[it].id }) { i -> JobRow(doneJobs[i]) }
        if (statuses.isNotEmpty() && jobs.isNotEmpty()) item { StatusSection("Needs") }
        if (statuses.isEmpty() && jobs.isEmpty()) {
            item {
                Column(Modifier.fillMaxWidth().padding(horizontal = 32.dp, vertical = 48.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                    Icon(Icons.Rounded.Campaign, null, tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(40.dp))
                    Spacer(Modifier.height(12.dp))
                    Text("No agent needs anything right now.", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(6.dp))
                    Text("When an agent needs something its machine lacks, its lead posts a status here, and every agent on every machine can offer help.",
                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = androidx.compose.ui.text.style.TextAlign.Center)
                }
            }
        }
        if (open.isNotEmpty() && jobs.isEmpty()) item { StatusSection("Open") }
        items(open.size, key = { "o" + open[it].id }) { i -> StatusRow(open[i], model) { onView(open, i) } }
        if (closed.isNotEmpty()) item { StatusSection("Recently closed") }
        items(closed.size, key = { "c" + closed[it].id }) { i -> StatusRow(closed[i], model) { onView(closed, i) } }
    }
}

@Composable
private fun StatusSection(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(start = 20.dp, top = 18.dp, bottom = 6.dp))
}

@Composable
private fun StatusRow(st: AgentStatus, model: AppModel, onClick: () -> Unit) {
    val seenTick by model.seenStatuses.collectAsState()
    val seen = remember(st.id, seenTick) { model.statusSeen(st.id) }
    Row(Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        RingAvatar(st, model, seen)
        Spacer(Modifier.width(14.dp))
        Column(Modifier.weight(1f)) {
            Text(st.forName, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(st.text, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
            val picked = st.offers.firstOrNull { it.member == st.picked }?.name
            val tail = when {
                !st.open && picked != null -> "Went with $picked"
                !st.open -> "Closed"
                st.offers.isEmpty() -> "No offers yet"
                st.offers.size == 1 -> "1 offer"
                else -> "${st.offers.size} offers"
            }
            Text(listOf(st.machine, statusAge(st.at), tail).filter { it.isNotEmpty() }.joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
    }
}

private const val StepMillis = 6000

/**
 * The full-screen viewer: one segment per status across the top, advancing
 * every few seconds; hold to pause, tap the left or right side to step.
 */
@Composable
fun StatusViewer(list: List<AgentStatus>, start: Int, model: AppModel, onClose: () -> Unit) {
    BackHandler(onBack = onClose)
    var index by remember { mutableIntStateOf(start.coerceIn(0, (list.size - 1).coerceAtLeast(0))) }
    var held by remember { mutableStateOf(false) }
    val progress = remember { Animatable(0f) }
    val st = list.getOrNull(index) ?: run { onClose(); return }
    LaunchedEffect(st.id) { model.markStatusSeen(st.id); progress.snapTo(0f) }
    LaunchedEffect(st.id, held) {
        if (held) { progress.stop(); return@LaunchedEffect }
        val left = ((1f - progress.value) * StepMillis).toInt()
        progress.animateTo(1f, tween(left, easing = LinearEasing))
        if (index < list.lastIndex) index++ else onClose()
    }
    val field = lerp(memberColor(model.memberForStatus(st.forId)?.id ?: st.forId), Color.Black, 0.62f)
    Box(
        Modifier.fillMaxSize().background(field)
            .pointerInput(st.id) {
                awaitEachGesture {
                    val down = awaitFirstDown()
                    held = true
                    val up = waitForUpOrCancellation()
                    held = false
                    // A short tap steps; a hold only pauses.
                    if (up != null && up.uptimeMillis - down.uptimeMillis < 250) {
                        if (down.position.x < size.width / 3f) { if (index > 0) index-- } else if (index < list.lastIndex) index++ else onClose()
                    }
                }
            },
    ) {
        Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding()) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                list.indices.forEach { i ->
                    val p = when { i < index -> 1f; i == index -> progress.value; else -> 0f }
                    Canvas(Modifier.weight(1f).height(3.dp)) {
                        drawRoundRect(Color.White.copy(alpha = 0.3f), size = size, cornerRadius = androidx.compose.ui.geometry.CornerRadius(2f))
                        drawRoundRect(Color.White, size = Size(size.width * p, size.height), cornerRadius = androidx.compose.ui.geometry.CornerRadius(2f))
                    }
                }
            }
            Row(Modifier.fillMaxWidth().padding(end = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClose) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back", tint = Color.White) }
                RingAvatar(st, model, seen = true, size = 44)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text(st.forName, color = Color.White, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    Text(listOf(st.roomName, st.machine, statusAge(st.at)).filter { it.isNotEmpty() }.joinToString(" · "),
                        color = Color.White.copy(alpha = 0.75f), style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
            Column(Modifier.weight(1f).fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 24.dp), verticalArrangement = Arrangement.Center) {
                Spacer(Modifier.height(24.dp))
                Text(if (st.open) "NEEDS" else if (st.picked.isNotEmpty()) "SORTED" else "CLOSED",
                    color = Color.White.copy(alpha = 0.7f), style = MaterialTheme.typography.labelLarge, fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.height(8.dp))
                Text(st.text, color = Color.White, fontSize = 30.sp, lineHeight = 38.sp, fontWeight = FontWeight.Medium)
                Spacer(Modifier.height(28.dp))
                Text(if (st.offers.isEmpty()) "No offers yet" else "Offers", color = Color.White.copy(alpha = 0.7f), style = MaterialTheme.typography.labelLarge)
                Spacer(Modifier.height(6.dp))
                st.offers.forEach { o -> OfferRow(o, picked = o.member == st.picked) { held = true; model.openStatusChat(o.room) } }
                Spacer(Modifier.height(24.dp))
            }
            Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(
                    Modifier.weight(1f).height(52.dp).clip(RoundedCornerShape(26.dp)).background(Color.Black.copy(alpha = 0.35f))
                        .clickable { onClose(); model.replyToStatus(st) }.padding(horizontal = 18.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(Icons.AutoMirrored.Rounded.Reply, null, tint = Color.White)
                    Spacer(Modifier.width(10.dp))
                    Text("Reply in ${st.roomName.ifEmpty { "the group" }}", color = Color.White, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
                if (st.open) {
                    Box(
                        Modifier.size(52.dp).clip(CircleShape).background(Color.Black.copy(alpha = 0.35f)).clickable { model.closeStatus(st.id); onClose() },
                        contentAlignment = Alignment.Center,
                    ) { Icon(Icons.Rounded.Close, "Close this status", tint = Color.White) }
                }
            }
        }
    }
}

@Composable
private fun OfferRow(o: StatusOffer, picked: Boolean, onOpen: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(vertical = 4.dp).clip(RoundedCornerShape(12.dp))
            .background(Color.White.copy(alpha = if (picked) 0.22f else 0.12f)).clickable(onClick = onOpen).padding(horizontal = 14.dp, vertical = 10.dp),
    ) {
        Text(listOf(o.name, o.machine).filter { it.isNotEmpty() }.joinToString(" · ") + if (picked) "  ✓ picked" else "",
            color = Color.White, style = MaterialTheme.typography.labelLarge, maxLines = 1, overflow = TextOverflow.Ellipsis)
        Text(o.text, color = Color.White.copy(alpha = 0.85f), style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
    }
}

/** A job: its state as a chip, who holds it on which machine, and its latest step. */
@Composable
private fun JobRow(j: io.github.phantomconartist.mace.data.Job) {
    val (label, color) = when (j.state) {
        "open" -> "Unclaimed" to MaterialTheme.colorScheme.tertiary
        "claimed" -> "Working" to mace.online
        "review" -> "In review" to mace.signal
        else -> "Approved" to MaterialTheme.colorScheme.onSurfaceVariant
    }
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.Top) {
        Text(j.id, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.width(52.dp).padding(top = 3.dp))
        Column(Modifier.weight(1f)) {
            Text(j.title, fontSize = 16.sp, maxLines = 2, overflow = TextOverflow.Ellipsis)
            val who = if (j.ownerName.isNotEmpty()) "${j.ownerName} on ${j.machine}" else if (j.needs.isNotEmpty()) "Needs " + j.needs.joinToString(", ") else j.roomName
            Text(listOf(who, statusAge(j.updated.ifEmpty { j.at })).filter { it.isNotEmpty() }.joinToString(" · "),
                style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            val step = if (j.state == "review" || j.state == "approved") j.proof else j.progress
            if (step.isNotEmpty()) Text(step, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            j.services.forEach { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.primary, maxLines = 1) }
        }
        Spacer(Modifier.width(8.dp))
        Text(label, style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold, color = color,
            modifier = Modifier.clip(RoundedCornerShape(6.dp)).background(color.copy(alpha = 0.14f)).padding(horizontal = 7.dp, vertical = 2.dp))
    }
}
