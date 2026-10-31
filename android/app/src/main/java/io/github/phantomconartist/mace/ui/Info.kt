package io.github.phantomconartist.mace.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.AddAPhoto
import androidx.compose.material.icons.rounded.Badge
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.Memory
import androidx.compose.material.icons.rounded.Pause
import androidx.compose.material.icons.rounded.PersonAdd
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.PowerSettingsNew
import androidx.compose.material.icons.rounded.RecordVoiceOver
import androidx.compose.material.icons.rounded.Stop
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.RadioButtonDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SecondaryTabRow
import androidx.compose.material3.Tab
import androidx.compose.material3.TabRowDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Message
import io.github.phantomconartist.mace.data.Policy
import io.github.phantomconartist.mace.data.Room
import io.github.phantomconartist.mace.ui.theme.mace

/** What the info page asks of the app. */
class InfoActions(
    val onBack: () -> Unit,
    val onMessage: (String) -> Unit,
    val onFiles: (String) -> Unit,
    val onPickPhoto: (String, Boolean) -> Unit,
    val onOpenFile: (String) -> Unit,
)

/** The profile page of an agent or a group, laid out like a messenger's contact and group info. */
@Composable
fun InfoPage(id: String, model: AppModel, actions: InfoActions) {
    BackHandler(onBack = actions.onBack)
    val rooms by model.rooms.collectAsState()
    val agents by model.agents.collectAsState()
    val observe by model.observe.collectAsState()
    val chat by model.chat.collectAsState()
    val room = rooms[id]
    val agent = agents.firstOrNull { it.id == id } ?: model.memberOf(id)
    var policy by remember(id) { mutableStateOf<Policy?>(null) }
    var reload by remember { mutableIntStateOf(0) }
    var files by remember(id) { mutableStateOf<List<Message>>(emptyList()) }
    var editing by remember { mutableStateOf(false) }
    var mode by remember { mutableStateOf(false) }
    var adding by remember { mutableStateOf(false) }
    var role by remember { mutableStateOf<Member?>(null) }
    var photoMenu by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(id, reload) { policy = runCatching { model.policy(id) }.getOrNull() }
    LaunchedEffect(id) { files = runCatching { model.sharedFiles(id) }.getOrDefault(emptyList()) }
    val name = room?.name ?: agent?.name ?: id.substringAfter("::")
    val group = room != null
    val photo = if (room != null) model.media.groupPhotoUrl(id, room.photo) else agent?.let { model.media.avatarUrl(it) }
    val paused = chat?.takeIf { it.id == id }?.run == "paused"

    Scaffold(containerColor = MaterialTheme.colorScheme.surface, topBar = { PageBar("", onBack = actions.onBack) }) { pad ->
        var tab by rememberSaveable { mutableIntStateOf(0) }
        LazyColumn(Modifier.padding(pad).fillMaxSize(), contentPadding = PaddingValues(bottom = 32.dp)) {
            item {
                Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally) {
                    Box(Modifier.clip(CircleShape).clickable { photoMenu = true }) {
                        Avatar(name, id, group, 140, photo, model.media)
                        DropdownMenu(photoMenu, { photoMenu = false }) {
                            DropdownMenuItem({ Text(if (photo == null) "Add photo" else "Change photo") }, leadingIcon = { Icon(Icons.Rounded.AddAPhoto, null) },
                                onClick = { photoMenu = false; actions.onPickPhoto(id, group) })
                            if (photo != null) DropdownMenuItem({ Text("Remove photo") }, leadingIcon = { Icon(Icons.Rounded.Delete, null) }, onClick = { photoMenu = false; model.removePhoto(id, group) })
                        }
                    }
                    Spacer(Modifier.height(14.dp))
                    Text(name, fontSize = 26.sp, textAlign = TextAlign.Center, modifier = Modifier.padding(horizontal = 24.dp))
                    Text(
                        if (room != null) "Group · ${room.members.size} ${if (room.members.size == 1) "agent" else "agents"}"
                        else listOfNotNull(agent?.handle?.takeIf { it.isNotEmpty() }?.let { "@$it" }, agent?.provider, agent?.machine ?: "this hub").joinToString(" · "),
                        fontSize = 16.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    val goal = policy?.goal.orEmpty()
                    if (group) {
                        Text(if (goal.isNotEmpty()) goal else "Set how this group works", fontSize = 16.sp, color = MaterialTheme.colorScheme.primary, textAlign = TextAlign.Center,
                            maxLines = 3, overflow = TextOverflow.Ellipsis, modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp).clickable { mode = true })
                    } else if (!agent?.role.isNullOrEmpty()) {
                        Text(agent!!.role, fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, textAlign = TextAlign.Center, modifier = Modifier.padding(horizontal = 24.dp, vertical = 6.dp))
                    }
                    Spacer(Modifier.height(12.dp))
                    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp), horizontalArrangement = Arrangement.SpaceEvenly) {
                        QuickButton(Icons.AutoMirrored.Rounded.Chat, "Message") { actions.onMessage(id) }
                        if (group) {
                            QuickButton(Icons.Rounded.Tune, "Mode") { mode = true }
                            QuickButton(if (paused) Icons.Rounded.PlayArrow else Icons.Rounded.Pause, if (paused) "Continue" else "Pause") { model.setPausedFor(id, !paused) }
                            QuickButton(Icons.Rounded.PersonAdd, "Add") { adding = true }
                        } else if (agent != null) {
                            QuickButton(Icons.Rounded.Folder, "Files") { actions.onFiles(agent.folder) }
                            QuickButton(Icons.Rounded.Edit, "Edit") { editing = true }
                            val stopped = agent.status == "stopped" || agent.status == "terminated"
                            QuickButton(if (stopped) Icons.Rounded.PlayArrow else Icons.Rounded.Stop, if (stopped) "Resume" else "Stop") { model.control(agent.id, if (stopped) "resume" else "stop"); reload++ }
                        }
                    }
                    Spacer(Modifier.height(16.dp))
                    HorizontalDivider(thickness = 6.dp, color = MaterialTheme.colorScheme.surfaceContainerLow)
                }
            }
            if (room != null) {
                item {
                    SecondaryTabRow(selectedTabIndex = tab, containerColor = MaterialTheme.colorScheme.surface,
                        indicator = { TabRowDefaults.SecondaryIndicator(Modifier.tabIndicatorOffset(tab, matchContentSize = true), height = 3.dp, color = mace.signal) },
                        divider = {}) {
                        listOf("Members", "Media", "Settings").forEachIndexed { i, t ->
                            Tab(tab == i, { tab = i }, text = { Text(t, fontSize = 16.sp, color = if (tab == i) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant) })
                        }
                    }
                }
                when (tab) {
                    0 -> {
                        item { Text("${room.members.size} members", fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 20.dp, vertical = 12.dp)) }
                        item { ActionRow(Icons.Rounded.PersonAdd, "Add members") { adding = true } }
                        items(room.members, key = { it }) { m ->
                            val a = agents.firstOrNull { it.id == m } ?: model.memberOf(m)
                            MemberRow(m, a, model, isLead = policy?.mode == "team" && policy?.lead == m, role = model.roleOf(id, m), onMessage = { actions.onMessage(m) },
                                onRole = { role = a ?: Member(m, m.substringAfter("::"), "", "", "", null) }, onRemove = { model.removeMember(id, m) })
                        }
                        if (room.members.isEmpty()) item { Text("No agents in this group. Add one to start working together.", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(20.dp)) }
                    }
                    1 -> item { MediaGrid(files, model.media, actions.onOpenFile) }
                    else -> {
                        item { SectionLabel("Settings") }
                        item { SettingRow(Icons.Rounded.Tune, "Conversation mode", policy?.let { modeLabel(it, true) }, onClick = { mode = true }) }
                        item { SettingRow(Icons.Rounded.RecordVoiceOver, "Texting style", if (policy?.tone == "professional") "Professional: short and plain" else "Casual: short, chat-style", onClick = { mode = true }) }
                        item { SettingRow(Icons.Rounded.Visibility, "Observe view", if (observe) "Shows tool calls and file edits" else "Messages only", onClick = { model.setObserve(!observe) }) { MaceSwitch(observe, model::setObserve) } }
                        item { HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, modifier = Modifier.padding(vertical = 8.dp)) }
                        item { SettingRow(Icons.Rounded.Delete, "Remove group", danger = true, onClick = { confirm = "group" }) }
                        item { Text("Removing a group stops its work and keeps its history. Removing a member keeps their agent and direct chat.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp)) }
                    }
                }
            } else if (agent != null) {
                item { MediaStrip(files, model.media, actions.onOpenFile) }
                item { SettingRow(Icons.Rounded.Badge, "Name, mention and role", "${agent.name} · @${agent.handle}" + if (agent.role.isNotEmpty()) " · ${agent.role}" else "", onClick = { editing = true }) }
                item { SettingRow(Icons.Rounded.Tune, "Conversation mode", policy?.let { modeLabel(it, false) }, onClick = { mode = true }) }
                item { SettingRow(Icons.Rounded.Visibility, "Observe view", if (observe) "Shows tool calls and file edits" else "Messages only", onClick = { model.setObserve(!observe) }) { MaceSwitch(observe, model::setObserve) } }
                item { SettingRow(Icons.Rounded.Memory, "Model", agent.model.ifEmpty { "CLI default" } + ". Change it with /model in the chat.") }
                item { SettingRow(Icons.Rounded.Folder, "Working folder", agent.folder.ifEmpty { "/" }, onClick = { actions.onFiles(agent.folder) }) }
                item { HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant, modifier = Modifier.padding(vertical = 8.dp)) }
                if (agent.status != "terminated") item { SettingRow(Icons.Rounded.PowerSettingsNew, "Terminate agent", "Ends its session. Resume brings it back.", danger = true, onClick = { model.control(agent.id, "terminate") }) }
                item { SettingRow(Icons.Rounded.Delete, "Remove agent from chats", "History stays on the hub. It also leaves every group.", danger = true, onClick = { confirm = "agent" }) }
            }
        }
    }
    if (editing && agent != null) ProfileDialog(agent, onSave = { n, h, r -> model.saveProfile(agent.id, n, h, r); editing = false }, onDismiss = { editing = false })
    if (mode) PolicySheet(id, room, agents, policy ?: Policy(mode = if (group) "team" else "reply"), onSave = { model.savePolicy(id, it); policy = it; mode = false }, onDismiss = { mode = false })
    if (adding && room != null) AddMemberDialog(agents.filter { it.id !in room.members }, model, onPick = { model.addMember(id, it.id); adding = false }, onDismiss = { adding = false })
    role?.let { m -> RoleDialog(m.name, room?.roles?.get(m.id).orEmpty(), onSave = { model.setRole(id, m.id, it); role = null }, onDismiss = { role = null }) }
    when (confirm) {
        "group" -> Confirm("Remove this group?", "Its work stops. The history stays on the hub.", "Remove", { model.removeConversation(id); actions.onBack() }) { confirm = null }
        "agent" -> Confirm("Remove ${agent?.name ?: "this agent"}?", "It leaves every chat and group. The history stays on the hub.", "Remove", { model.removeConversation(id); actions.onBack() }) { confirm = null }
    }
}

private fun modeLabel(p: Policy, group: Boolean): String = when (p.mode) {
    "team" -> "Goal: a lead runs the team until done"
    "goal" -> "Goal oriented" + if (p.goal.isNotEmpty()) ": " + p.goal else ""
    "continuous" -> "Until done"
    else -> (if (group) "Free: casual chat, " else "Reply limit: ") + if (p.limit == 1) "1 reply" else "${p.limit} replies"
}

@Composable
private fun QuickButton(icon: ImageVector, label: String, onClick: () -> Unit) {
    Column(Modifier.clip(RoundedCornerShape(16.dp)).clickable(onClick = onClick).padding(4.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Box(Modifier.size(width = 76.dp, height = 56.dp).clip(RoundedCornerShape(28.dp)).background(MaterialTheme.colorScheme.surfaceContainerHigh), contentAlignment = Alignment.Center) {
            Icon(icon, null, tint = MaterialTheme.colorScheme.onSurface)
        }
        Spacer(Modifier.height(6.dp))
        Text(label, fontSize = 14.sp)
    }
}

@Composable
private fun MemberRow(id: String, a: Member?, model: AppModel, isLead: Boolean, role: String, onMessage: () -> Unit, onRole: () -> Unit, onRemove: () -> Unit) {
    var menu by remember { mutableStateOf(false) }
    Box {
        Row(Modifier.fillMaxWidth().clickable { menu = true }.padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Avatar(a?.name ?: id, id, false, 48, a?.let { model.media.avatarUrl(it) }, model.media)
            Spacer(Modifier.width(16.dp))
            Column(Modifier.weight(1f)) {
                Text(a?.name ?: "Agent not reachable", fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (a == null) Text("Removed, or on a machine that is offline", fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                else Text(shortRole(role).ifEmpty { "Add a role" }, fontSize = 15.sp, color = if (role.isEmpty()) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            if (isLead) {
                Text("Lead", fontSize = 13.sp, color = MaterialTheme.colorScheme.onPrimaryContainer,
                    modifier = Modifier.clip(RoundedCornerShape(6.dp)).background(MaterialTheme.colorScheme.primaryContainer).padding(horizontal = 8.dp, vertical = 4.dp))
            }
        }
        DropdownMenu(menu, { menu = false }) {
            DropdownMenuItem({ Text("Message ${a?.name ?: ""}".trim()) }, onClick = { menu = false; onMessage() })
            DropdownMenuItem({ Text("Set role in this group") }, onClick = { menu = false; onRole() })
            DropdownMenuItem({ Text("Stop") }, onClick = { menu = false; model.control(id, "stop") })
            DropdownMenuItem({ Text("Terminate") }, onClick = { menu = false; model.control(id, "terminate") })
            DropdownMenuItem({ Text("Remove from group", color = MaterialTheme.colorScheme.error) }, onClick = { menu = false; onRemove() })
        }
    }
}

/** A thumbnail of a shared file: the picture, a video frame, or its kind. */
@Composable
private fun Thumb(path: String, media: MediaLoader, modifier: Modifier, onOpen: (String) -> Unit) {
    var img by remember(path) { mutableStateOf<ImageBitmap?>(null) }
    LaunchedEffect(path) {
        img = when (kindOf(path)) {
            FileKind.Image -> media.image(media.fileUrl(path), 360)
            FileKind.Video -> media.videoFrame(path, 360)
            else -> null
        }
    }
    Box(modifier.clip(RoundedCornerShape(6.dp)).background(MaterialTheme.colorScheme.surfaceContainerHigh).clickable { onOpen(path) }, contentAlignment = Alignment.Center) {
        img?.let { Image(it, null, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize()) } ?: Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.padding(6.dp)) {
            Icon(if (path.endsWith("/")) Icons.Rounded.Folder else Icons.Rounded.InsertDriveFile, null, tint = MaterialTheme.colorScheme.primary)
            Text(path.trimEnd('/').substringAfterLast('/'), fontSize = 11.sp, maxLines = 2, overflow = TextOverflow.Ellipsis, textAlign = TextAlign.Center)
        }
        if (kindOf(path) == FileKind.Video) Icon(Icons.Rounded.PlayArrow, null, tint = androidx.compose.ui.graphics.Color.White, modifier = Modifier.align(Alignment.BottomStart).padding(4.dp))
    }
}

/** "Media, links, and docs": the files shared in this chat, newest first. */
@Composable
private fun MediaStrip(files: List<Message>, media: MediaLoader, onOpen: (String) -> Unit) {
    if (files.isEmpty()) return
    Column {
        Row(Modifier.fillMaxWidth().padding(start = 20.dp, end = 16.dp, top = 16.dp, bottom = 10.dp), verticalAlignment = Alignment.CenterVertically) {
            Text("Media, links, and docs", fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
            Text(files.size.toString(), fontSize = 15.sp, color = MaterialTheme.colorScheme.onSurfaceVariant)
            Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        LazyRow(contentPadding = PaddingValues(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            items(files.map { it.file }.distinct(), key = { it }) { f -> Thumb(f, media, Modifier.size(92.dp), onOpen) }
        }
        Spacer(Modifier.height(12.dp))
        HorizontalDivider(thickness = 6.dp, color = MaterialTheme.colorScheme.surfaceContainerLow)
    }
}

@Composable
private fun MediaGrid(files: List<Message>, media: MediaLoader, onOpen: (String) -> Unit) {
    val paths = files.map { it.file }.distinct()
    if (paths.isEmpty()) {
        Text("Nothing shared yet. Pictures, videos, and files sent in this group appear here.", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(20.dp))
        return
    }
    Column(Modifier.padding(8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        paths.chunked(3).forEach { row ->
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                row.forEach { Thumb(it, media, Modifier.weight(1f).aspectRatio(1f), onOpen) }
                repeat(3 - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}

/** Renames an agent completely: display name, @mention handle, and its role everywhere. */
@Composable
fun ProfileDialog(a: Member, onSave: (String, String, String) -> Unit, onDismiss: () -> Unit) {
    var name by remember { mutableStateOf(a.name) }
    var handle by remember { mutableStateOf(a.handle) }
    var role by remember { mutableStateOf(a.role) }
    val clean = handle.lowercase().replace(Regex("[^a-z0-9._-]"), "-").trim('-', '.', '_')
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Name, mention and role") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                OutlinedTextField(name, { name = it.take(60) }, label = { Text("Name") }, singleLine = true)
                OutlinedTextField(handle, { handle = it.removePrefix("@").take(32) }, label = { Text("Mention") }, prefix = { Text("@") }, singleLine = true,
                    supportingText = { Text(if (clean.isNotEmpty()) "Mention it as @$clean in groups." else "Letters, numbers, dots, and dashes.") })
                OutlinedTextField(role, { role = it.take(2000) }, label = { Text("Role in every chat (optional)") }, placeholder = { Text("For example, reviews the mobile layouts") }, maxLines = 4)
            }
        },
        confirmButton = { TextButton({ onSave(name, clean, role) }, enabled = name.isNotBlank() && clean.isNotEmpty()) { Text("Save") } },
        dismissButton = { TextButton(onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun RoleDialog(name: String, current: String, onSave: (String) -> Unit, onDismiss: () -> Unit) {
    var role by remember { mutableStateOf(current) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("$name's role here") },
        text = {
            Column {
                OutlinedTextField(role, { role = it.take(2000) }, label = { Text("Role (optional)") }, placeholder = { Text("For example, review the mobile layouts") }, maxLines = 4)
                Text("Applies only in this group, from their next turn. Leave it empty and they can choose one as they work.", style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 8.dp))
            }
        },
        confirmButton = { TextButton({ onSave(role) }) { Text("Save role") } },
        dismissButton = { TextButton(onDismiss) { Text("Cancel") } },
    )
}

@Composable
private fun AddMemberDialog(candidates: List<Member>, model: AppModel, onPick: (Member) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Add an agent") },
        text = {
            if (candidates.isEmpty()) Text("Every agent is already here. Start another from the Agents tab.")
            else LazyColumn { items(candidates, key = { it.id }) { a -> AgentRow(a, model.media) { onPick(a) } } }
        },
        confirmButton = { TextButton(onDismiss) { Text("Close") } },
    )
}

/** How the agents in a conversation work, the same choices as on the desktop. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PolicySheet(id: String, room: Room?, agents: List<Member>, initial: Policy, onSave: (Policy) -> Unit, onDismiss: () -> Unit) {
    var p by remember(initial) { mutableStateOf(initial.copy(mode = if (room != null && initial.mode !in listOf("team", "reply", "goal", "continuous")) "team" else initial.mode)) }
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = MaterialTheme.colorScheme.surfaceContainer) {
        Column(Modifier.verticalScroll(rememberScrollState()).navigationBarsPadding().padding(horizontal = 20.dp).padding(bottom = 24.dp)) {
            Text("How should agents work?", style = MaterialTheme.typography.titleLarge)
            Spacer(Modifier.height(8.dp))
            val options = if (room != null) listOf("team" to "Goal: a lead runs the team until done", "reply" to "Free: casual chat") +
                (if (p.mode == "goal" || p.mode == "continuous") listOf(p.mode to (if (p.mode == "goal") "Goal oriented (older)" else "Until done (older)")) else emptyList())
            else listOf("reply" to "Reply limit", "goal" to "Goal oriented")
            options.forEach { (m, label) -> Radio(label, p.mode == m) { p = p.copy(mode = m) } }
            if (room != null && p.mode == "team") {
                Text("Lead", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
                room.members.forEach { m -> Radio(agents.firstOrNull { it.id == m }?.name ?: m.substringAfter("::"), p.lead == m) { p = p.copy(lead = m) } }
            }
            if (room != null) {
                Text("Texting style", style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(top = 12.dp, bottom = 4.dp))
                Radio("Casual: short, chat-style, shorthand welcome", p.tone != "professional") { p = p.copy(tone = "casual") }
                Radio("Professional: short and plain", p.tone == "professional") { p = p.copy(tone = "professional") }
            }
            if (p.mode == "goal") OutlinedTextField(p.goal, { p = p.copy(goal = it) }, label = { Text("Goal") }, minLines = 2, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            if (p.mode != "continuous" && p.mode != "team") {
                OutlinedTextField(p.limit.toString(), { v -> v.filter(Char::isDigit).toIntOrNull()?.let { p = p.copy(limit = it.coerceIn(1, 100)) } }, label = { Text("Maximum agent replies") },
                    singleLine = true, modifier = Modifier.fillMaxWidth().padding(top = 8.dp))
            }
            Text(
                when (p.mode) {
                    "team" -> "Your messages go to the lead unless you @mention someone. The lead plans, assigns roles, directs the others, and says done when everything works."
                    "continuous" -> "After your message, agents keep working together until one reports it done, they run out of things to add, or you step in."
                    "goal" -> "Stops on an agent completion report or the reply budget. Review its evidence."
                    else -> "Agents take turns within this total reply budget."
                },
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(vertical = 12.dp),
            )
            Button({ onSave(p) }, enabled = !(p.mode == "team" && p.lead.isEmpty()) && !(p.mode == "goal" && p.goal.isBlank()),
                colors = ButtonDefaults.buttonColors(containerColor = mace.signal, contentColor = mace.onSignal), modifier = Modifier.fillMaxWidth().height(50.dp)) { Text("Save mode") }
        }
    }
}

@Composable
private fun Radio(label: String, selected: Boolean, onClick: () -> Unit) {
    Row(Modifier.fillMaxWidth().clip(RoundedCornerShape(8.dp)).clickable(onClick = onClick).padding(vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        RadioButton(selected, onClick, colors = RadioButtonDefaults.colors(selectedColor = mace.signal))
        Spacer(Modifier.width(4.dp))
        Text(label, fontSize = 16.sp)
    }
}
