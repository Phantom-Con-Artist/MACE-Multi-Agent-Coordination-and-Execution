package io.github.phantomconartist.mace.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.gestures.detectHorizontalDragGestures
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
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
import androidx.compose.material.icons.rounded.Build
import androidx.compose.material.icons.rounded.ChatBubbleOutline
import androidx.compose.material.icons.rounded.ContentCopy
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.ExpandLess
import androidx.compose.material.icons.rounded.ExpandMore
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material.icons.rounded.Done
import androidx.compose.material.icons.rounded.DoneAll
import androidx.compose.material.icons.rounded.ErrorOutline
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.Info
import androidx.compose.material.icons.rounded.KeyboardDoubleArrowDown
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Pause
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.RestartAlt
import androidx.compose.material.icons.rounded.Schedule
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.derivedStateOf
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
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.platform.LocalConfiguration
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.SpanStyle
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.buildAnnotatedString
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.text.withStyle
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.ChatState
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Message
import io.github.phantomconartist.mace.ui.theme.mace
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import kotlin.math.roundToInt
import kotlinx.coroutines.launch

// The conversation follows the consumer messenger model from the research
// (WhatsApp, Messenger): tailed bubbles with the time on the last line,
// ticks for sent, delivered, and read, an unread marker, date chips,
// presence in the header, swipe to reply, and a composer in the thumb zone.

/** A message the owner is replying to, quoted above the composer. */
data class Quote(val sender: String, val name: String, val text: String)

private val timeFormat = DateTimeFormatter.ofPattern("h:mm a")

private fun local(at: String) = runCatching { OffsetDateTime.parse(at).atZoneSameInstant(ZoneId.systemDefault()) }.getOrNull()

private fun dayLabel(day: LocalDate): String = when {
    day == LocalDate.now() -> "Today"
    day == LocalDate.now().minusDays(1) -> "Yesterday"
    day.isAfter(LocalDate.now().minusDays(7)) -> day.dayOfWeek.getDisplayName(java.time.format.TextStyle.FULL, java.util.Locale.getDefault())
    else -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.MEDIUM))
}

/** Each run of messages from one sender within a minute shares one tail. */
private fun startsRun(prev: Message?, m: Message): Boolean {
    if (prev == null || prev.sender != m.sender || prev.kind == "system" || m.kind == "system" || prev.sender == "mace") return true
    val a = local(prev.at) ?: return true
    val b = local(m.at) ?: return true
    return a.toLocalDate() != b.toLocalDate() || java.time.Duration.between(a, b).seconds > 60
}

/** What a chat needs from the hub besides its state. */
class ChatActions(
    val onBack: () -> Unit,
    val onInfo: () -> Unit,
    val onMode: () -> Unit,
    val onSend: (String) -> Unit,
    val onSendFile: (String, String) -> Unit,
    /** Picks from the phone: pictures and videos when true, any file otherwise. */
    val onUpload: (Boolean) -> Unit,
    val onRetry: (Message) -> Unit,
    val onOlder: () -> Unit,
    val onInserted: () -> Unit,
    val onCommand: (String, String) -> Unit,
    val onPause: (Boolean) -> Unit,
    val onControl: (String, String) -> Unit,
    val onOpenFile: (String) -> Unit,
    val onObserve: (Boolean) -> Unit,
    val onLink: () -> Unit,
    /** Opens the hub's file browser on the web. */
    val onBrowse: () -> Unit,
    val files: suspend (String) -> List<io.github.phantomconartist.mace.data.Entry>,
    val models: suspend (String) -> io.github.phantomconartist.mace.data.Models,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ChatScreen(
    state: ChatState,
    memberOf: (String) -> Member?,
    roleOf: (String) -> String,
    media: MediaLoader,
    observe: Boolean,
    linked: Boolean,
    actions: ChatActions,
) {
    var quote by remember(state.id) { mutableStateOf<Quote?>(null) }
    var attachment by rememberSaveable(state.id) { mutableStateOf("") }
    var attaching by remember { mutableStateOf(false) }
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
    LaunchedEffect(wantsModels) { if (wantsModels && modelList == null) modelList = runCatching { actions.models(agent!!.id) }.getOrNull() }
    val cd = if (agent != null) Regex("""^/cd\s+(.*)$""").find(draft.text)?.groupValues?.get(1) else null
    val cdDir = cd?.let { cdTarget(it, agent!!.folder).first }
    LaunchedEffect(cdDir) {
        if (cdDir != null && folderList?.first != cdDir) {
            folderList = cdDir to runCatching { actions.files(cdDir.trim('/').ifEmpty { "." }) }.getOrDefault(emptyList()).filter { it.directory && !it.name.startsWith(".") }.map { it.name }.sorted()
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
            "files" -> actions.onBrowse()
            "new" -> confirmNew = true
            else -> actions.onCommand(name, arg)
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
            actions.onInserted()
        }
    }
    // Newest messages sit at the bottom; reaching the top loads older ones.
    LaunchedEffect(list, state.messages.size) {
        snapshotFlow { list.layoutInfo.visibleItemsInfo.lastOrNull()?.index }.collect { last ->
            if (last != null && last >= list.layoutInfo.totalItemsCount - 3 && state.older && !state.loading) actions.onOlder()
        }
    }
    // A new message of the owner's own scrolls into view.
    LaunchedEffect(state.messages.lastOrNull()?.id) {
        if (state.messages.lastOrNull()?.sender == "you") list.animateScrollToItem(0)
    }
    val away by remember { derivedStateOf { list.firstVisibleItemIndex > 2 } }

    fun reply(m: Message) {
        val name = if (m.sender == "you") "You" else memberOf(m.sender)?.name ?: "Agent"
        val first = m.text.lineSequence().firstOrNull { it.isNotBlank() && !it.startsWith(">") }.orEmpty()
        quote = Quote(m.sender, name, first.ifEmpty { m.file.trimEnd('/').substringAfterLast('/') }.take(140))
        val handle = memberOf(m.sender)?.handle
        if (state.group && handle != null && !draft.text.contains("@$handle")) {
            val next = "@$handle " + draft.text
            draft = TextFieldValue(next, TextRange(next.length))
        }
    }

    Column(Modifier.fillMaxSize().background(MaterialTheme.colorScheme.surface)) {
        TopAppBar(
            colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
            navigationIcon = { IconButton(onClick = actions.onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back") } },
            title = {
                Row(Modifier.clip(RoundedCornerShape(24.dp)).clickable(onClick = actions.onInfo).padding(end = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Avatar(state.name, state.id, state.group, 40, agent?.let { media.avatarUrl(it) }, media)
                    Spacer(Modifier.width(12.dp))
                    Column {
                        Text(state.name, style = MaterialTheme.typography.titleMedium, fontSize = 18.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
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
                if (state.group) IconButton(onClick = actions.onMode) { Icon(Icons.Rounded.Tune, "Conversation mode") }
                else if (agent != null) IconButton(onClick = actions.onBrowse) { Icon(Icons.Rounded.Folder, "Browse files on the web") }
                IconButton(onClick = { menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                DropdownMenu(menu, { menu = false }) {
                    DropdownMenuItem({ Text(if (state.group) "Group info" else "Agent info") }, leadingIcon = { Icon(Icons.Rounded.Info, null) }, onClick = { menu = false; actions.onInfo() })
                    DropdownMenuItem({ Text("Conversation mode") }, leadingIcon = { Icon(Icons.Rounded.Tune, null) }, onClick = { menu = false; actions.onMode() })
                    DropdownMenuItem(
                        { Text(if (observe) "Messenger view" else "Observe view") },
                        leadingIcon = { Icon(if (observe) Icons.Rounded.ChatBubbleOutline else Icons.Rounded.Visibility, null) },
                        onClick = { menu = false; actions.onObserve(!observe) },
                    )
                    if (state.run == "paused") DropdownMenuItem({ Text("Continue") }, leadingIcon = { Icon(Icons.Rounded.PlayArrow, null) }, onClick = { menu = false; actions.onPause(false) })
                    else DropdownMenuItem({ Text("Pause") }, leadingIcon = { Icon(Icons.Rounded.Pause, null) }, onClick = { menu = false; actions.onPause(true) })
                    agent?.let { a ->
                        if (a.status == "stopped" || a.status == "terminated") {
                            DropdownMenuItem({ Text("Resume agent") }, leadingIcon = { Icon(Icons.Rounded.PlayArrow, null) }, onClick = { menu = false; actions.onControl(a.id, "resume") })
                        } else {
                            DropdownMenuItem({ Text("Stop agent") }, leadingIcon = { Icon(Icons.Rounded.Stop, null) }, onClick = { menu = false; actions.onControl(a.id, "stop") })
                        }
                        DropdownMenuItem({ Text("Start a fresh session") }, leadingIcon = { Icon(Icons.Rounded.RestartAlt, null) }, onClick = { menu = false; confirmNew = true })
                    }
                }
            },
        )
        if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())

        Column(Modifier.weight(1f).fillMaxWidth().background(mace.chat).chatPattern(mace.pattern)) {
            if (state.run == "paused") {
                SystemNote("Paused. Write to give new direction, or continue from the menu.")
            }
            val down = if (state.group) state.members.filter { !it.online } else state.members.ifEmpty { null }?.filter { !it.online } ?: listOf(null)
            if (down.isNotEmpty() && !state.loading) {
                SystemNote(
                    when {
                        !state.group && agent?.reachable == false || agent == null && !state.group -> "This agent's machine is offline. The history is here; it can answer once the machine is back."
                        !state.group -> "This agent is ${agent?.status}. Resume it from the menu to continue."
                        down.size == state.members.size -> "No agent in this group can answer right now."
                        else -> down.joinToString(", ") { it?.name.orEmpty() } + if (down.size == 1) " is offline." else " are offline."
                    },
                )
            }
            Box(Modifier.weight(1f).fillMaxWidth()) {
                val messages = state.messages
                val unread = if (state.firstUnread > 0) messages.count { it.seq >= state.firstUnread && it.sender != "you" } else 0
                val bubbleMax = (LocalConfiguration.current.screenWidthDp * 0.82f).dp
                LazyColumn(Modifier.fillMaxSize(), state = list, reverseLayout = true, contentPadding = PaddingValues(top = 8.dp, bottom = 6.dp)) {
                    // Working agents show what they are doing, below the last message.
                    state.working.forEach { (member, steps) ->
                        item(key = "working-$member") { WorkingRow(member, memberOf(member)?.name ?: "Agent", steps, state.group, observe) }
                    }
                    for (i in messages.indices.reversed()) {
                        val m = messages[i]
                        val prev = messages.getOrNull(i - 1)
                        item(key = m.id) {
                            Column {
                                val day = local(m.at)?.toLocalDate()
                                if (day != null && day != prev?.let { local(it.at)?.toLocalDate() }) DayChip(dayLabel(day))
                                if (m.seq > 0 && m.seq == state.firstUnread && unread > 0) UnreadMarker(unread)
                                MessageRow(m, startsRun(prev, m), state, bubbleMax, memberOf, roleOf, media, observe, actions.onOpenFile, onRetry = actions.onRetry, onReply = { reply(m) })
                            }
                        }
                    }
                    if (!state.loading && messages.isEmpty()) {
                        item { SystemNote(if (state.group) "Say what the team should do. Mention an agent with @." else "Say hello. Type / for this agent's commands.") }
                    }
                }
                androidx.compose.animation.AnimatedVisibility(away, enter = scaleIn() + fadeIn(), exit = scaleOut() + fadeOut(), modifier = Modifier.align(Alignment.BottomEnd).padding(12.dp)) {
                    Surface(shape = CircleShape, color = MaterialTheme.colorScheme.surfaceContainerHigh, shadowElevation = 3.dp, modifier = Modifier.size(40.dp).clickable { scope.launch { list.animateScrollToItem(0) } }) {
                        Box(contentAlignment = Alignment.Center) { Icon(Icons.Rounded.KeyboardDoubleArrowDown, "Latest messages", tint = MaterialTheme.colorScheme.onSurfaceVariant) }
                    }
                }
            }
            state.problem?.let {
                Text(it, color = MaterialTheme.colorScheme.onErrorContainer, style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 8.dp, vertical = 4.dp).clip(RoundedCornerShape(12.dp))
                        .background(MaterialTheme.colorScheme.errorContainer).padding(horizontal = 14.dp, vertical = 8.dp))
            }
            if (!linked) {
                Row(
                    Modifier.fillMaxWidth().navigationBarsPadding().padding(8.dp).clip(RoundedCornerShape(26.dp)).background(MaterialTheme.colorScheme.surfaceContainerHigh)
                        .clickable(onClick = actions.onLink).padding(horizontal = 18.dp, vertical = 14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Text("Saved chat. Link to your hub to send messages.", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
                    Text("Link", color = MaterialTheme.colorScheme.primary, style = MaterialTheme.typography.labelLarge)
                }
            } else Composer(
                draft = draft,
                quote = quote,
                attachment = attachment,
                suggestions = suggestions,
                group = state.group,
                onDraft = { draft = it },
                onPick = { o ->
                    draft = draft.withText(o.text)
                    if (o.run) runCommand(o.text)
                },
                onClearQuote = { quote = null },
                onAttach = { attaching = true },
                onClearAttachment = { attachment = "" },
                onSend = send@{
                    if (attachment.isEmpty() && quote == null && runCommand(draft.text)) return@send
                    val q = quote
                    val text = if (q != null) "> ${q.name}: ${q.text}\n${draft.text}" else draft.text
                    if (attachment.isNotEmpty()) actions.onSendFile(text, attachment) else actions.onSend(text)
                    draft = TextFieldValue("")
                    quote = null
                    attachment = ""
                    scope.launch { list.animateScrollToItem(0) }
                },
            )
        }
    }
    if (attaching) AttachSheet(agent?.folder, onPick = { attaching = false; picker = it }, onPhone = { attaching = false; actions.onUpload(it) }, onDismiss = { attaching = false })
    picker?.let { start ->
        FilePicker(start, actions.files, onPick = { attachment = it; picker = null }, onDismiss = { picker = null })
    }
    if (confirmNew) {
        androidx.compose.material3.AlertDialog(
            onDismissRequest = { confirmNew = false },
            title = { Text("Start a fresh session?") },
            text = { Text("The agent forgets this conversation's native memory. The messages stay here.") },
            confirmButton = { androidx.compose.material3.TextButton({ confirmNew = false; actions.onCommand("new", "") }) { Text("Start fresh") } },
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
    if (state.group) return state.members.joinToString(", ") { it.name }.ifEmpty { "No agents yet" }
    val m = state.members.firstOrNull() ?: return "Offline: its machine is down"
    if (!m.reachable) return "Offline: ${m.machine ?: "its machine"} is down"
    // Working shows from live activity above; a stale "replying" status reads as online.
    return listOfNotNull(statusWord(m.status).takeIf { m.status != "replying" } ?: "Online", m.provider.ifEmpty { null }, m.machine ?: "this hub").joinToString(" · ")
}

@Composable
private fun DayChip(text: String) {
    Box(Modifier.fillMaxWidth().padding(vertical = 8.dp), contentAlignment = Alignment.Center) {
        Surface(shape = RoundedCornerShape(8.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            Text(text, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = 12.dp, vertical = 6.dp))
        }
    }
}

/** A note from MACE: centered, like a messenger's system notes; longer ones keep their lines. */
@Composable
private fun SystemNote(text: String) {
    val lines = '\n' in text.trim()
    Box(Modifier.fillMaxWidth().padding(horizontal = 28.dp, vertical = 4.dp), contentAlignment = Alignment.Center) {
        Surface(shape = RoundedCornerShape(10.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            androidx.compose.foundation.text.selection.SelectionContainer {
                Text(text.trim(), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = if (lines) TextAlign.Start else TextAlign.Center, modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp))
            }
        }
    }
}

@Composable
private fun UnreadMarker(count: Int) {
    Box(Modifier.fillMaxWidth().padding(vertical = 8.dp), contentAlignment = Alignment.Center) {
        Surface(shape = RoundedCornerShape(16.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh) {
            Text(if (count == 1) "1 unread message" else "$count unread messages", style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp))
        }
    }
}

/** An agent replying: an incoming bubble with its current step and three pulsing dots. */
@Composable
private fun WorkingRow(id: String, name: String, steps: List<io.github.phantomconartist.mace.data.Step>, group: Boolean, observe: Boolean) {
    val step = if (observe) "" else steps.lastOrNull()?.title.orEmpty()
    val pulse = rememberInfiniteTransition(label = "typing")
    Row(Modifier.fillMaxWidth().padding(start = 6.dp, end = 48.dp, top = 6.dp, bottom = 2.dp)) {
        Surface(shape = BubbleShape(mine = false, tail = true), color = mace.incoming, contentColor = mace.onIncoming) {
            Column(Modifier.padding(start = BubbleTail + 10.dp, end = 12.dp, top = 8.dp, bottom = 9.dp)) {
                if (group) Text(name, style = MaterialTheme.typography.labelLarge, color = memberColor(id))
                if (observe) StepList(steps.takeLast(6), mace.onIncoming)
                Row(verticalAlignment = Alignment.CenterVertically) {
                    repeat(3) { i ->
                        val a by pulse.animateFloat(0.25f, 1f, infiniteRepeatable(tween(600, delayMillis = i * 160), RepeatMode.Reverse), label = "dot$i")
                        Box(Modifier.padding(end = 4.dp).size(7.dp).graphicsLayer { alpha = a }.clip(CircleShape).background(mace.onIncoming))
                    }
                    if (step.isNotEmpty()) {
                        Spacer(Modifier.width(6.dp))
                        Text(step, style = MaterialTheme.typography.bodyMedium, fontStyle = FontStyle.Italic, color = mace.onIncoming.copy(alpha = 0.7f),
                            maxLines = 2, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun MessageRow(
    m: Message,
    first: Boolean,
    state: ChatState,
    bubbleMax: Dp,
    memberOf: (String) -> Member?,
    roleOf: (String) -> String,
    media: MediaLoader,
    observe: Boolean,
    onOpenFile: (String) -> Unit,
    onRetry: (Message) -> Unit,
    onReply: () -> Unit,
) {
    if (m.sender == "mace" || m.kind == "system") {
        SystemNote(m.text)
        return
    }
    val mine = m.sender == "you"
    val haptics = LocalHapticFeedback.current
    val clipboard = LocalClipboardManager.current
    val density = LocalDensity.current
    val threshold = with(density) { 48.dp.toPx() }
    val drag = remember { Animatable(0f) }
    val scope = rememberCoroutineScope()
    var menu by remember { mutableStateOf(false) }
    var armed by remember { mutableStateOf(false) }
    val showAvatar = state.group && !mine

    Box(
        Modifier.fillMaxWidth().padding(top = if (first) 6.dp else 2.dp)
            // Swipe right to reply: elastic, with a haptic tick past the threshold.
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
                    val next = (drag.value + amount * 0.6f).coerceIn(0f, threshold * 1.6f)
                    if (!armed && next >= threshold) { armed = true; haptics.performHapticFeedback(HapticFeedbackType.SegmentTick) }
                    if (armed && next < threshold) armed = false
                    scope.launch { drag.snapTo(next) }
                }
            },
    ) {
        Box(
            Modifier.align(Alignment.CenterStart).padding(start = 16.dp).size(32.dp)
                .graphicsLayer { alpha = (drag.value / threshold).coerceIn(0f, 1f); scaleX = alpha; scaleY = alpha }
                .clip(CircleShape).background(MaterialTheme.colorScheme.surfaceContainerHigh),
            contentAlignment = Alignment.Center,
        ) { Icon(Icons.AutoMirrored.Rounded.Reply, null, tint = MaterialTheme.colorScheme.onSurface, modifier = Modifier.size(18.dp)) }
        Row(
            Modifier.fillMaxWidth().offset { IntOffset(drag.value.roundToInt(), 0) }
                .padding(start = if (mine) 48.dp else 6.dp, end = if (mine) 6.dp else 40.dp),
            horizontalArrangement = if (mine) Arrangement.End else Arrangement.Start,
        ) {
            if (showAvatar) {
                Box(Modifier.width(34.dp)) {
                    if (first) {
                        val who = memberOf(m.sender)
                        Avatar(who?.name ?: m.sender.substringAfter("::"), m.sender, false, 30, who?.let { media.avatarUrl(it) }, media)
                    }
                }
            }
            Box {
                val shape = BubbleShape(mine, first)
                val ink = if (mine) mace.onOutgoing else mace.onIncoming
                Surface(
                    shape = shape,
                    color = if (mine) mace.outgoing else mace.incoming,
                    contentColor = ink,
                    shadowElevation = 0.5.dp,
                    modifier = Modifier.widthIn(max = bubbleMax).clip(shape).combinedClickable(
                        onClick = { if (m.kind == "failed") onRetry(m) },
                        onLongClick = { haptics.performHapticFeedback(HapticFeedbackType.LongPress); menu = true },
                    ),
                ) {
                    val time = local(m.at)?.format(timeFormat)?.lowercase().orEmpty()
                    val meta = time
                    val reserve = "   " + meta + if (mine) "     " else ""
                    Column(Modifier.padding(start = if (mine) 6.dp else BubbleTail + 6.dp, end = if (mine) BubbleTail + 6.dp else 6.dp, top = 4.dp, bottom = 6.dp)) {
                        if (!mine && first && state.group) {
                            Row(Modifier.padding(horizontal = 4.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
                                Text(memberOf(m.sender)?.name ?: m.sender.substringAfter("::"), style = MaterialTheme.typography.labelLarge,
                                    color = memberColor(m.sender), maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                                roleOf(m.sender).takeIf { it.isNotEmpty() }?.let {
                                    Spacer(Modifier.width(12.dp))
                                    Text(it, style = MaterialTheme.typography.labelMedium, color = ink.copy(alpha = 0.6f), maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                            }
                        }
                        if (observe && m.activity.isNotEmpty()) StepsDisclosure(m.activity, ink)
                        Box {
                            Body(m, reserve, ink, bubbleMax, media, onOpenFile)
                            // On a picture without a caption the time sits on a dark scrim.
                            val onPicture = m.text.isBlank() && (kindOf(m.file) == FileKind.Image || kindOf(m.file) == FileKind.Video)
                            val metaInk = if (onPicture) Color.White else ink
                            Row(
                                Modifier.align(Alignment.BottomEnd).padding(end = 4.dp, bottom = if (onPicture) 6.dp else 0.dp)
                                    .then(if (onPicture) Modifier.padding(end = 4.dp).clip(RoundedCornerShape(10.dp)).background(Color(0x73000000)).padding(horizontal = 6.dp, vertical = 2.dp) else Modifier),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Text(meta, fontSize = 11.sp, color = metaInk.copy(alpha = if (onPicture) 0.95f else 0.65f))
                                if (mine) {
                                    Spacer(Modifier.width(3.dp))
                                    Ticks(m, state, metaInk)
                                }
                            }
                        }
                        if (m.kind == "failed") {
                            Text("Not sent. Tap to try again.", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(horizontal = 4.dp))
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

/** A quoted reply ("> Name: text" on the first line) shows as a quote card, as in a messenger. */
private val quoteLine = Regex("""^> ([^:\n]{1,80}): (.*)$""")

@Composable
private fun Body(m: Message, reserve: String, ink: Color, width: Dp, media: MediaLoader, onOpenFile: (String) -> Unit) {
    var text = m.text
    quoteLine.find(text.lineSequence().firstOrNull().orEmpty())?.let { q ->
        text = text.substringAfter('\n', "")
        Row(Modifier.padding(bottom = 4.dp).height(IntrinsicSize.Min).clip(RoundedCornerShape(8.dp)).background(ink.copy(alpha = 0.08f))) {
            Box(Modifier.width(4.dp).fillMaxHeight().background(MaterialTheme.colorScheme.primary))
            Column(Modifier.padding(horizontal = 8.dp, vertical = 5.dp)) {
                Text(q.groupValues[1], style = MaterialTheme.typography.labelLarge, color = MaterialTheme.colorScheme.primary)
                Text(q.groupValues[2], style = MaterialTheme.typography.bodySmall, color = ink.copy(alpha = 0.75f), maxLines = 3, overflow = TextOverflow.Ellipsis)
            }
        }
    }
    val captioned = text.isNotBlank()
    if (m.file.isNotEmpty()) FileCard(m.file, ink, width, media, onOpenFile)
    if (captioned) Box(Modifier.padding(horizontal = 4.dp, vertical = 2.dp)) { Markdown(text, ink, reserve = reserve) }
    else if (kindOf(m.file) == FileKind.Other || kindOf(m.file) == FileKind.Folder) {
        Text(buildAnnotatedString { withStyle(SpanStyle(color = Color.Transparent, fontSize = 11.sp)) { append(reserve) } }, modifier = Modifier.padding(horizontal = 4.dp))
    }
}

/** Clock while sending; one tick on the hub; two when delivered; two in blue once an agent read it. */
@Composable
private fun Ticks(m: Message, state: ChatState, ink: Color) {
    val size = Modifier.size(16.dp)
    val muted = ink.copy(alpha = 0.6f)
    when {
        m.kind == "failed" -> Icon(Icons.Rounded.ErrorOutline, "Not sent", modifier = size, tint = MaterialTheme.colorScheme.error)
        m.seq == 0L -> Icon(Icons.Rounded.Schedule, "Sending", modifier = size, tint = muted)
        state.receipts.values.any { it >= m.seq } -> Icon(Icons.Rounded.DoneAll, "Read", modifier = size, tint = mace.read)
        m.delivery.values.any { it == "delivered" } -> Icon(Icons.Rounded.DoneAll, "Delivered", modifier = size, tint = muted)
        else -> Icon(Icons.Rounded.Done, "Sent", modifier = size, tint = muted)
    }
}

/** In observe view, the steps an agent took for a reply: collapsed to a count, open to the list. */
@Composable
private fun StepsDisclosure(steps: List<io.github.phantomconartist.mace.data.Step>, ink: Color) {
    var open by remember { mutableStateOf(false) }
    Column(Modifier.padding(horizontal = 4.dp, vertical = 2.dp).clip(RoundedCornerShape(8.dp)).background(ink.copy(alpha = 0.07f)).clickable { open = !open }.padding(horizontal = 8.dp, vertical = 6.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Rounded.Build, null, tint = ink.copy(alpha = 0.7f), modifier = Modifier.size(14.dp))
            Spacer(Modifier.width(6.dp))
            val edits = steps.count { it.kind == "edit" || it.kind == "write" }
            Text(listOfNotNull("${steps.size} steps", if (edits > 0) "$edits file edits" else null).joinToString(" · "), style = MaterialTheme.typography.labelMedium, color = ink.copy(alpha = 0.75f))
            Spacer(Modifier.weight(1f, fill = false).width(8.dp))
            Icon(if (open) Icons.Rounded.ExpandLess else Icons.Rounded.ExpandMore, if (open) "Hide steps" else "Show steps", tint = ink.copy(alpha = 0.6f), modifier = Modifier.size(18.dp))
        }
        AnimatedVisibility(open) { StepList(steps, ink) }
    }
}

/** Tool calls and file edits, one line each, with what they touched. */
@Composable
private fun StepList(steps: List<io.github.phantomconartist.mace.data.Step>, ink: Color) {
    Column(Modifier.padding(top = 4.dp, bottom = 4.dp)) {
        steps.forEach { st ->
            Row(Modifier.padding(vertical = 3.dp), verticalAlignment = Alignment.Top) {
                Icon(
                    when (st.kind) {
                        "edit", "write" -> Icons.Rounded.Edit
                        "read" -> Icons.Rounded.Description
                        "command", "shell", "exec" -> Icons.Rounded.Terminal
                        "search" -> Icons.Rounded.Search
                        else -> Icons.Rounded.Build
                    },
                    null, tint = if (st.state == "failed") MaterialTheme.colorScheme.error else ink.copy(alpha = 0.6f), modifier = Modifier.padding(top = 2.dp).size(14.dp),
                )
                Spacer(Modifier.width(8.dp))
                Column {
                    Text(st.title.ifEmpty { st.kind }, style = MaterialTheme.typography.bodySmall, color = ink.copy(alpha = 0.9f), maxLines = 2, overflow = TextOverflow.Ellipsis)
                    if (st.detail.isNotBlank()) Text(st.detail.trim(), fontFamily = androidx.compose.ui.text.font.FontFamily.Monospace, fontSize = 11.sp, color = ink.copy(alpha = 0.6f), maxLines = 3, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}
