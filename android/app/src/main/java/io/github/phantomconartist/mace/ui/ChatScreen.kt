package io.github.phantomconartist.mace.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Reply
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.Done
import androidx.compose.material.icons.rounded.DoneAll
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Pause
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.snapshotFlow
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.Shape
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import io.github.phantomconartist.mace.ChatState
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Message
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlin.math.roundToInt
import kotlinx.coroutines.launch

// The conversation follows the consumer messenger model from the research
// (WhatsApp, Messenger): bubbles with a tail on the first message of each
// run, ticks for sent, delivered, and read, a "New" divider, date chips,
// presence in the header, swipe to reply, and a composer in the thumb zone.

/** A message the owner is replying to, quoted above the composer. */
data class Quote(val sender: String, val name: String, val text: String)

private val timeFormat = DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT)

private fun local(at: String) = runCatching { OffsetDateTime.parse(at).atZoneSameInstant(ZoneId.systemDefault()) }.getOrNull()

private fun dayLabel(day: LocalDate): String = when (day) {
    LocalDate.now() -> "Today"
    LocalDate.now().minusDays(1) -> "Yesterday"
    else -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}

/** Each run of messages from one sender within a minute shares one tail. */
private fun startsRun(prev: Message?, m: Message): Boolean {
    if (prev == null || prev.sender != m.sender || prev.kind == "system" || m.kind == "system") return true
    val a = local(prev.at) ?: return true
    val b = local(m.at) ?: return true
    return java.time.Duration.between(a, b).seconds > 60
}

/** Agent colors for names and avatars in groups, from the theme's own roles. */
@Composable
private fun memberColor(id: String): Color {
    val c = MaterialTheme.colorScheme
    val palette = listOf(c.primary, c.tertiary, c.secondary, c.error)
    return palette[(id.hashCode() and Int.MAX_VALUE) % palette.size]
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    state: ChatState,
    memberOf: (String) -> Member?,
    onBack: () -> Unit,
    onSend: (String) -> Unit,
    onRetry: (Message) -> Unit,
    onOlder: () -> Unit,
    onInserted: () -> Unit,
    onCommand: (String, String) -> Unit,
    onPause: (Boolean) -> Unit,
    onControl: (String, String) -> Unit,
    files: suspend (String) -> List<io.github.phantomconartist.mace.data.Entry>,
    models: suspend (String) -> io.github.phantomconartist.mace.data.Models,
    onSendFile: (String, String) -> Unit,
) {
    var quote by remember(state.id) { mutableStateOf<Quote?>(null) }
    var attachment by rememberSaveable(state.id) { mutableStateOf("") }
    var picker by remember { mutableStateOf<String?>(null) }
    var menu by remember { mutableStateOf(false) }
    var confirmNew by remember { mutableStateOf(false) }
    val agent = if (state.group) null else state.members.firstOrNull()
    var modelList by remember(state.id) { mutableStateOf<io.github.phantomconartist.mace.data.Models?>(null) }
    var folderList by remember(state.id) { mutableStateOf<Pair<String, List<String>>?>(null) }
    var draft by rememberSaveable(state.id, stateSaver = TextFieldValue.Saver) { mutableStateOf(TextFieldValue("")) }
    val list = rememberLazyListState()
    val scope = rememberCoroutineScope()

    // Models and folders load when the draft asks for them.
    val wantsModels = agent != null && draft.text.startsWith("/model ")
    LaunchedEffect(wantsModels) { if (wantsModels && modelList == null) modelList = runCatching { models(agent!!.id) }.getOrNull() }
    val cd = if (agent != null) Regex("""^/cd\s+(.*)$""").find(draft.text)?.groupValues?.get(1) else null
    val cdDir = cd?.let { cdTarget(it, agent!!.folder).first }
    LaunchedEffect(cdDir) {
        if (cdDir != null && folderList?.first != cdDir) {
            folderList = cdDir to runCatching { files(cdDir.trim('/').ifEmpty { "." }) }.getOrDefault(emptyList()).filter { it.directory && !it.name.startsWith(".") }.map { it.name }.sorted()
        }
    }
    val suggestions = when {
        state.group -> mentionSuggestions(draft, state.members)
        agent != null && draft.text.startsWith("/") -> commandSuggestions(draft.text, modelList, folderList, agent.folder)
        else -> emptyList()
    }

    fun runCommand(text: String): Boolean {
        if (agent == null) return false
        val (name, arg) = parseCommand(text) ?: return false
        when (name) {
            "files" -> picker = agent.folder
            "new" -> confirmNew = true
            else -> onCommand(name, arg)
        }
        draft = TextFieldValue("")
        return true
    }

    LaunchedEffect(state.insert) {
        state.insert?.let { text ->
            if (!draft.text.contains(text.trim())) {
                val next = text + draft.text
                draft = TextFieldValue(next, TextRange(next.length))
            }
            onInserted()
        }
    }
    // Newest messages sit at the bottom; reaching the top loads older ones.
    LaunchedEffect(list, state.messages.size) {
        snapshotFlow { list.layoutInfo.visibleItemsInfo.lastOrNull()?.index }.collect { last ->
            if (last != null && last >= list.layoutInfo.totalItemsCount - 3 && state.older && !state.loading) onOlder()
        }
    }
    // A new message of the owner's own scrolls into view.
    LaunchedEffect(state.messages.lastOrNull()?.id) {
        if (state.messages.lastOrNull()?.sender == "you") list.animateScrollToItem(0)
    }

    fun reply(m: Message) {
        val name = if (m.sender == "you") "You" else memberOf(m.sender)?.name ?: "Agent"
        quote = Quote(m.sender, name, m.text.lineSequence().firstOrNull { it.isNotBlank() }.orEmpty().take(140))
        val handle = memberOf(m.sender)?.handle
        if (state.group && handle != null && !draft.text.contains("@$handle")) {
            val next = "@$handle " + draft.text
            draft = TextFieldValue(next, TextRange(next.length))
        }
    }

    Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface)) {
        TopAppBar(
            navigationIcon = { IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back") } },
            title = {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Avatar(state.name, state.id, state.group, 40)
                    Spacer(Modifier.width(12.dp))
                    Column {
                        Text(state.name, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(
                            presence(state, memberOf),
                            style = MaterialTheme.typography.bodySmall,
                            color = if (state.working.isNotEmpty()) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                    }
                }
            },
            actions = {
                IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "Conversation controls") }
                DropdownMenu(menu, { menu = false }) {
                    if (state.run == "paused") DropdownMenuItem({ Text("Continue") }, leadingIcon = { Icon(Icons.Rounded.PlayArrow, null) }, onClick = { menu = false; onPause(false) })
                    else DropdownMenuItem({ Text("Pause this conversation") }, leadingIcon = { Icon(Icons.Rounded.Pause, null) }, onClick = { menu = false; onPause(true) })
                    agent?.let { a ->
                        if (a.status == "stopped" || a.status == "terminated") {
                            DropdownMenuItem({ Text("Resume agent") }, leadingIcon = { Icon(Icons.Rounded.PlayArrow, null) }, onClick = { menu = false; onControl(a.id, "resume") })
                        } else {
                            DropdownMenuItem({ Text("Stop agent") }, leadingIcon = { Icon(Icons.Rounded.Stop, null) }, onClick = { menu = false; onControl(a.id, "stop") })
                        }
                        DropdownMenuItem({ Text("Browse files") }, leadingIcon = { Icon(Icons.Rounded.Folder, null) }, onClick = { menu = false; picker = a.folder })
                    }
                }
            },
        )
        if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())
        if (state.run == "paused") {
            Text("Paused. Write to give new direction, or continue from the menu.", style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSecondaryContainer,
                modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.secondaryContainer).padding(horizontal = 16.dp, vertical = 6.dp))
        }

        val messages = state.messages
        LazyColumn(Modifier.weight(1f).fillMaxWidth(), state = list, reverseLayout = true, contentPadding = androidx.compose.foundation.layout.PaddingValues(vertical = 8.dp)) {
            // Working agents show what they are doing, below the last message.
            state.working.forEach { (member, steps) ->
                item(key = "working-$member") { WorkingRow(memberOf(member)?.name ?: "Agent", steps.lastOrNull()?.title.orEmpty(), state.group) }
            }
            for (i in messages.indices.reversed()) {
                val m = messages[i]
                val prev = messages.getOrNull(i - 1)
                item(key = m.id) {
                    Column {
                        val day = local(m.at)?.toLocalDate()
                        if (day != null && day != prev?.let { local(it.at)?.toLocalDate() }) DayChip(dayLabel(day))
                        if (m.seq > 0 && m.seq == state.firstUnread) NewDivider()
                        MessageRow(m, startsRun(prev, m), state, memberOf, onRetry = onRetry, onReply = { reply(m) })
                    }
                }
            }
            if (!state.loading && messages.isEmpty()) {
                item { DayChip(if (state.group) "Say what the team should do." else "Say hello. Messages go to the agent on your hub.") }
            }
        }

        state.problem?.let {
            Text(it, color = MaterialTheme.colorScheme.onErrorContainer, style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.errorContainer).padding(horizontal = 16.dp, vertical = 8.dp))
        }
        Composer(
            draft = draft,
            quote = quote,
            attachment = attachment,
            suggestions = suggestions,
            onDraft = { draft = it },
            onPick = { o ->
                draft = draft.withText(o.text)
                if (o.run) runCommand(o.text)
            },
            onClearQuote = { quote = null },
            onAttach = { picker = agent?.folder ?: "." },
            onClearAttachment = { attachment = "" },
            onSend = send@{
                if (attachment.isEmpty() && quote == null && runCommand(draft.text)) return@send
                val q = quote
                val text = if (q != null) "> ${q.name}: ${q.text}\n${draft.text}" else draft.text
                if (attachment.isNotEmpty()) onSendFile(text, attachment) else onSend(text)
                draft = TextFieldValue("")
                quote = null
                attachment = ""
                scope.launch { list.animateScrollToItem(0) }
            },
        )
    }
    picker?.let { start ->
        FilePicker(start, files, onPick = { attachment = it; picker = null }, onDismiss = { picker = null })
    }
    if (confirmNew) {
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirmNew = false },
            title = { Text("Start a fresh session?") },
            text = { Text("The agent forgets this conversation's native memory. The messages stay here.") },
            confirmButton = { androidx.compose.material3.TextButton({ confirmNew = false; onCommand("new", "") }) { Text("Start fresh") } },
            dismissButton = { androidx.compose.material3.TextButton({ confirmNew = false }) { Text("Cancel") } },
        )
    }
}

private fun presence(state: ChatState, memberOf: (String) -> Member?): String {
    state.working.entries.firstOrNull()?.let { (member, steps) ->
        val who = if (state.group) (memberOf(member)?.name ?: "An agent") + " is " else ""
        val doing = steps.lastOrNull { it.state == "running" }?.title ?: steps.lastOrNull()?.title
        return who + (doing?.let { "working: " + it.replaceFirstChar { c -> c.lowercase() } } ?: "working…")
    }
    if (state.group) return state.members.joinToString(", ") { it.name }.ifEmpty { "Group" }
    val m = state.members.firstOrNull() ?: return ""
    return listOfNotNull(m.provider.ifEmpty { null }, m.machine ?: "this hub").joinToString(" · ")
}

@Composable
fun Avatar(name: String, id: String, group: Boolean, size: Int) {
    val color = memberColor(id)
    Box(
        Modifier.size(size.dp).clip(if (group) RoundedCornerShape((size / 3.5).dp) else CircleShape).background(color.copy(alpha = 0.18f)),
        contentAlignment = Alignment.Center,
    ) {
        Text(name.trim().take(1).uppercase(), color = color, fontWeight = FontWeight.SemiBold,
            style = if (size >= 40) MaterialTheme.typography.titleMedium else MaterialTheme.typography.labelSmall)
    }
}

@Composable
private fun DayChip(text: String) {
    Box(Modifier.fillMaxWidth().padding(vertical = 8.dp), contentAlignment = Alignment.Center) {
        Surface(shape = RoundedCornerShape(8.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, tonalElevation = 1.dp) {
            Text(text, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center, modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
        }
    }
}

/** A longer note from MACE, such as a command's answer, kept readable as lines. */
@Composable
private fun NoteCard(text: String) {
    Surface(shape = RoundedCornerShape(12.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.fillMaxWidth().padding(horizontal = 24.dp, vertical = 6.dp)) {
        androidx.compose.foundation.text.selection.SelectionContainer {
            Text(text.trim(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(14.dp))
        }
    }
}

@Composable
private fun NewDivider() {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.weight(1f).heightIn(min = 1.dp, max = 1.dp).background(MaterialTheme.colorScheme.primary))
        Text("New", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.primary, modifier = Modifier.padding(horizontal = 8.dp))
        Box(Modifier.weight(1f).heightIn(min = 1.dp, max = 1.dp).background(MaterialTheme.colorScheme.primary))
    }
}

@Composable
private fun WorkingRow(name: String, step: String, group: Boolean) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 12.dp, vertical = 4.dp)) {
        Surface(shape = RoundedCornerShape(18.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            Column(Modifier.padding(horizontal = 14.dp, vertical = 10.dp)) {
                if (group) Text(name, style = MaterialTheme.typography.labelLarge, color = memberColor(name))
                Text(step.ifEmpty { "Thinking…" }, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2, overflow = TextOverflow.Ellipsis)
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun MessageRow(m: Message, first: Boolean, state: ChatState, memberOf: (String) -> Member?, onRetry: (Message) -> Unit, onReply: () -> Unit) {
    if (m.sender == "mace" || m.kind == "system") {
        if ('\n' in m.text.trim()) NoteCard(m.text) else DayChip(m.text)
        return
    }
    val mine = m.sender == "you"
    val haptics = LocalHapticFeedback.current
    val clipboard = LocalClipboardManager.current
    val density = LocalDensity.current
    val threshold = with(density) { 40.dp.toPx() }
    val drag = remember { Animatable(0f) }
    val scope = rememberCoroutineScope()
    var menu by remember { mutableStateOf(false) }
    var armed by remember { mutableStateOf(false) }

    Box(
        Modifier.fillMaxWidth().padding(top = if (first) 6.dp else 2.dp)
            // Swipe right to reply: elastic, with a haptic tick past 40dp.
            .pointerInput(m.id) {
                detectHorizontalDragGestures(
                    onDragEnd = {
                        if (armed) onReply()
                        armed = false
                        scope.launch { drag.animateTo(0f) }
                    },
                    onDragCancel = { armed = false; scope.launch { drag.animateTo(0f) } },
                ) { change, amount ->
                    change.consume()
                    val next = (drag.value + amount * 0.6f).coerceIn(0f, threshold * 1.8f)
                    if (!armed && next >= threshold) { armed = true; haptics.performHapticFeedback(HapticFeedbackType.SegmentTick) }
                    if (armed && next < threshold) armed = false
                    scope.launch { drag.snapTo(next) }
                }
            },
    ) {
        Icon(
            Icons.AutoMirrored.Rounded.Reply, null,
            tint = MaterialTheme.colorScheme.primary,
            modifier = Modifier.align(Alignment.CenterStart).padding(start = 16.dp).size(22.dp)
                .graphicsLayer { alpha = (drag.value / threshold).coerceIn(0f, 1f); scaleX = alpha; scaleY = alpha },
        )
        Row(
            Modifier.fillMaxWidth().offset { IntOffset(drag.value.roundToInt(), 0) }.padding(horizontal = 8.dp),
            horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start,
        ) {
            val tail = 4.dp
            val shape: Shape = when {
                !first -> RoundedCornerShape(18.dp)
                mine -> RoundedCornerShape(18.dp, tail, 18.dp, 18.dp)
                else -> RoundedCornerShape(tail, 18.dp, 18.dp, 18.dp)
            }
            Box {
                Surface(
                    shape = shape,
                    color = if (mine) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surfaceContainerHigh,
                    contentColor = if (mine) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurface,
                    modifier = Modifier.widthIn(max = 340.dp).clip(shape).combinedClickable(
                        onClick = { if (m.kind == "failed") onRetry(m) },
                        onLongClick = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); menu = true },
                    ),
                ) {
                    Column(Modifier.padding(start = 12.dp, end = 12.dp, top = 8.dp, bottom = 6.dp)) {
                        if (!mine && first && state.group) {
                            Text(memberOf(m.sender)?.name ?: m.sender.substringAfter("::"), style = MaterialTheme.typography.labelLarge, color = memberColor(m.sender))
                        }
                        Body(m)
                        Row(Modifier.align(Alignment.End), verticalAlignment = Alignment.CenterVertically) {
                            if (m.activity.isNotEmpty()) {
                                Text("${m.activity.size} steps · ", style = MaterialTheme.typography.labelSmall, color = androidx.compose.material3.LocalContentColor.current.copy(alpha = 0.7f))
                            }
                            Text(local(m.at)?.format(timeFormat).orEmpty(), style = MaterialTheme.typography.labelSmall,
                                color = androidx.compose.material3.LocalContentColor.current.copy(alpha = 0.7f))
                            if (mine) {
                                Spacer(Modifier.width(4.dp))
                                Ticks(m, state)
                            }
                        }
                        if (m.kind == "failed") {
                            Text("Not sent. Tap to try again.", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.error)
                        }
                    }
                }
                DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                    DropdownMenuItem(text = { Text("Reply") }, leadingIcon = { Icon(Icons.AutoMirrored.Rounded.Reply, null) }, onClick = { menu = false; onReply() })
                    DropdownMenuItem(text = { Text("Copy") }, leadingIcon = { Icon(Icons.Rounded.ContentCopy, null) }, onClick = {
                        menu = false
                        clipboard.setText(AnnotatedString(m.text))
                    })
                }
            }
        }
    }
}

@Composable
private fun Body(m: Message) {
    val color = androidx.compose.material3.LocalContentColor.current
    if (m.file.isNotEmpty()) {
        Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.padding(bottom = 4.dp)) {
            Icon(if (m.file.endsWith("/")) Icons.Rounded.Folder else Icons.Rounded.InsertDriveFile, null, modifier = Modifier.size(20.dp))
            Spacer(Modifier.width(8.dp))
            Text(m.file.trimEnd('/').substringAfterLast('/'), style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.Medium)
        }
    }
    if (m.text.isNotBlank()) {
        if (m.sender == "you") Text(m.text, style = MaterialTheme.typography.bodyLarge, color = color)
        else Markdown(m.text, color)
    }
}

/** Clock while sending; one tick on the hub; two when delivered; two in the accent once an agent read it. */
@Composable
private fun Ticks(m: Message, state: ChatState) {
    val size = Modifier.size(16.dp)
    // Inside the owner's bubble: the bubble's own text color, faded; read stands out in full.
    val ink = androidx.compose.material3.LocalContentColor.current
    val muted = ink.copy(alpha = 0.6f)
    when {
        m.kind == "failed" -> Icon(Icons.Rounded.ErrorOutline, "Not sent", modifier = size, tint = MaterialTheme.colorScheme.error)
        m.seq == 0L -> Icon(Icons.Rounded.Schedule, "Sending", modifier = size, tint = muted)
        state.receipts.values.any { it >= m.seq } -> Icon(Icons.Rounded.DoneAll, "Read", modifier = size, tint = MaterialTheme.colorScheme.tertiary)
        m.delivery.values.any { it == "delivered" } -> Icon(Icons.Rounded.DoneAll, "Delivered", modifier = size, tint = muted)
        else -> Icon(Icons.Rounded.Done, "Sent", modifier = size, tint = muted)
    }
}
