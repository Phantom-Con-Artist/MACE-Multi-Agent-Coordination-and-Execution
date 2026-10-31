package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Logout
import androidx.compose.material.icons.rounded.ChatBubbleOutline
import androidx.compose.material.icons.automirrored.rounded.OpenInNew
import androidx.compose.material.icons.rounded.Apps
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.material.icons.rounded.FolderOpen
import androidx.compose.material.icons.rounded.History
import androidx.compose.material.icons.rounded.MonitorHeart
import androidx.compose.material.icons.rounded.OpenInBrowser
import androidx.compose.material.icons.rounded.DeleteSweep
import androidx.compose.material.icons.rounded.Devices
import androidx.compose.material.icons.rounded.Link
import androidx.compose.material.icons.rounded.NotificationsActive
import androidx.compose.material.icons.rounded.PhoneAndroid
import androidx.compose.material.icons.rounded.Visibility
import androidx.compose.material.icons.rounded.WorkHistory
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.BuildConfig
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Device
import io.github.phantomconartist.mace.data.Node
import io.github.phantomconartist.mace.ui.theme.mace
import kotlinx.coroutines.launch

/** A settings row: icon, title, and an optional line under it, with an optional control on the right. */
@Composable
fun SettingRow(icon: ImageVector, title: String, detail: String? = null, danger: Boolean = false, onClick: (() -> Unit)? = null, trailing: @Composable (() -> Unit)? = null) {
    Row(
        Modifier.fillMaxWidth().then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier).padding(horizontal = 24.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.width(28.dp))
        Column(Modifier.weight(1f)) {
            Text(title, fontSize = 17.sp, color = if (danger) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurface)
            detail?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant) }
        }
        trailing?.invoke()
    }
}

@Composable
fun MaceSwitch(checked: Boolean, onChange: (Boolean) -> Unit) {
    Switch(checked, onChange, colors = SwitchDefaults.colors(checkedTrackColor = mace.signal, checkedThumbColor = mace.onSignal))
}

/** Marks a row that opens in the web app. */
@Composable
fun WebMark() {
    Icon(Icons.AutoMirrored.Rounded.OpenInNew, "Opens on the web", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(20.dp))
}

private fun androidx.compose.ui.graphics.Color.toArgbInt() = android.graphics.Color.argb((alpha * 255).toInt(), (red * 255).toInt(), (green * 255).toInt(), (blue * 255).toInt())

/** An agent row for lists: avatar with photo, name, and where it runs. */
@Composable
fun AgentRow(a: Member, media: MediaLoader, trailing: @Composable (() -> Unit)? = null, onClick: () -> Unit) {
    Row(Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Box {
            Box(Modifier.clip(CircleShape)) { Avatar(a.name, a.id, false, 48, media.avatarUrl(a), media) }
            if (!a.online || a.status == "replying") {
                Box(Modifier.align(Alignment.BottomEnd).size(14.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surface).padding(2.dp).clip(CircleShape)
                    .background(if (a.online) mace.online else mace.down))
            }
        }
        Spacer(Modifier.width(16.dp))
        Column(Modifier.weight(1f)) {
            Text(a.name, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(listOfNotNull("@${a.handle}".takeIf { a.handle.isNotEmpty() }, if (!a.reachable) "Offline: its machine is down" else statusWord(a.status), a.role.ifEmpty { null } ?: a.machine ?: "this hub").joinToString(" · "),
                style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        trailing?.invoke()
    }
}

@Composable
fun AgentsTab(agents: List<Member>, model: AppModel, actions: HomeActions) {
    var query by remember { mutableStateOf("") }
    val q = query.trim().lowercase()
    val shown = agents.filter { q.isEmpty() || it.name.lowercase().contains(q) || it.handle.lowercase().contains(q) || it.provider.lowercase().contains(q) }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 96.dp)) {
        item { SearchPill(query, { query = it }, "Search agents") }
        if (agents.isEmpty()) item { Text("No agents yet. Start one on any of your machines.", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(24.dp)) }
        shown.groupBy { it.machine ?: "This hub" }.toSortedMap(compareBy { if (it == "This hub") "" else it }).forEach { (machine, list) ->
            item { SectionLabel(if (list.none { it.reachable }) "$machine · offline" else machine) }
            items(list, key = { it.id }) { a -> AgentRow(a, model.media, onClick = { actions.onOpen(a.id) }) }
        }
    }
}

@Composable
fun MachinesTab(model: AppModel) {
    val context = androidx.compose.ui.platform.LocalContext.current
    val bar = MaterialTheme.colorScheme.surface.toArgbInt()
    fun web(page: String) { model.webUrl(page)?.let { openWeb(context, it, bar) } }
    var nodes by remember { mutableStateOf<List<Node>?>(null) }
    var problem by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) { runCatching { model.nodes() }.onSuccess { nodes = it }.onFailure { problem = it.message } }
    LazyColumn(Modifier.fillMaxSize()) {
        item {
            Text("Every computer joined to your hub. Agents can run on any of them that is online.", style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 20.dp, vertical = 8.dp))
        }
        item { SettingRow(Icons.Rounded.MonitorHeart, "Monitor", "CPU, memory, and load, on the web", onClick = { web("host") }) { WebMark() } }
        item { SettingRow(Icons.Rounded.FolderOpen, "Files", "Browse and preview files, on the web", onClick = { web("files") }) { WebMark() } }
        item { SettingRow(Icons.Rounded.Apps, "Apps", "Open desktop apps on a machine, on the web", onClick = { web("apps") }) { WebMark() } }
        item { SettingRow(Icons.Rounded.History, "Activity", "Everything agents and you did, on the web", onClick = { web("activity") }) { WebMark() } }
        item { SectionLabel("Machines") }
        problem?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(20.dp)) } }
        if (nodes == null && problem == null) item { Text("Loading…", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(20.dp)) }
        items(nodes.orEmpty(), key = { it.id }) { n ->
            Row(Modifier.fillMaxWidth().clickable { web("hosts") }.padding(horizontal = 16.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(48.dp).clip(CircleShape).background(memberColor(n.id).copy(alpha = 0.2f)), contentAlignment = Alignment.Center) {
                    Icon(Icons.Rounded.Computer, null, tint = memberColor(n.id))
                }
                Spacer(Modifier.width(16.dp))
                Column(Modifier.weight(1f)) {
                    Text(n.name.ifEmpty { n.id } + if (n.self) " (this hub)" else "", fontSize = 17.sp)
                    Text(
                        when {
                            !n.self && !n.available -> if (n.lastSeen.isNotEmpty()) "Offline · last seen " + listTime(n.lastSeen) else "Offline"
                            n.problem.isNotEmpty() -> n.problem
                            n.outdated -> "Online · needs an update"
                            else -> "Online"
                        },
                        style = MaterialTheme.typography.bodyMedium,
                        color = if (n.self || n.available) mace.online else mace.down,
                        maxLines = 2, overflow = TextOverflow.Ellipsis,
                    )
                }
            }
        }
    }
}

@Composable
fun SettingsTab(hubName: String, model: AppModel, onSignOut: () -> Unit, onLinked: () -> Unit, onLink: () -> Unit) {
    val linked by model.linked.collectAsState()
    var confirmDelete by remember { mutableStateOf(false) }
    val observe by model.observe.collectAsState()
    val sounds by model.soundsOn.collectAsState()
    var keepWorking by remember { mutableStateOf<Boolean?>(null) }
    var devices by remember { mutableStateOf<List<Device>>(emptyList()) }
    var reload by remember { mutableIntStateOf(0) }
    var confirmOut by remember { mutableStateOf(false) }
    LaunchedEffect(reload) {
        keepWorking = runCatching { model.keepWorking() }.getOrNull()
        devices = runCatching { model.devices() }.getOrDefault(emptyList())
    }
    LazyColumn(Modifier.fillMaxSize()) {
        item {
            Row(Modifier.fillMaxWidth().padding(horizontal = 20.dp, vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.size(64.dp).clip(CircleShape).background(mace.signal), contentAlignment = Alignment.Center) {
                    Text("M", fontSize = 28.sp, color = mace.onSignal)
                }
                Spacer(Modifier.width(16.dp))
                Column {
                    Text(hubName.ifEmpty { "Your hub" }, fontSize = 20.sp)
                    Text("MACE ${BuildConfig.VERSION_NAME} · client API ${BuildConfig.MACE_API}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
        }
        item { SectionLabel("Chats") }
        item {
            SettingRow(Icons.Rounded.Visibility, "Observe view", if (observe) "Shows each agent's tool calls and file edits" else "Off: chats show messages only", onClick = { model.setObserve(!observe) }) {
                MaceSwitch(observe, model::setObserve)
            }
        }
        item {
            SettingRow(Icons.Rounded.NotificationsActive, "Message sounds", "A pop when you send, a chime when an agent answers", onClick = { model.setSounds(!sounds) }) {
                MaceSwitch(sounds, model::setSounds)
            }
        }
        item { SectionLabel("Work") }
        item {
            SettingRow(Icons.Rounded.WorkHistory, "Keep working when I leave",
                when (keepWorking) { true -> "Agents keep working with every device closed"; false -> "Work stops shortly after every device closes"; null -> "Unavailable" },
                onClick = { keepWorking?.let { model.setKeepWorking(!it); keepWorking = !it } }) {
                keepWorking?.let { k -> MaceSwitch(k) { model.setKeepWorking(it); keepWorking = it } }
            }
        }
        item {
            val context = androidx.compose.ui.platform.LocalContext.current
            val bar = MaterialTheme.colorScheme.surface.toArgbInt()
            SettingRow(Icons.Rounded.OpenInBrowser, "MACE on the web", "The full desktop workspace", onClick = { model.webUrl("home")?.let { openWeb(context, it, bar) } }) { WebMark() }
        }
        item { SectionLabel("Devices") }
        item { SettingRow(Icons.Rounded.Devices, "Linked devices", "${devices.size} signed in. Link a browser or another phone.", onClick = onLinked) }
        item { Spacer(Modifier.padding(8.dp)) }
        if (linked) item { SettingRow(Icons.AutoMirrored.Rounded.Logout, "Unlink from this hub", "Saved chats stay readable on this phone", danger = true, onClick = { confirmOut = true }) }
        else item { SettingRow(Icons.Rounded.Link, "Link to a hub", "Scan its QR code or type its link", onClick = onLink) }
        item { SettingRow(Icons.Rounded.DeleteSweep, "Delete saved chats", "Removes them from this phone. The hub keeps its history.", danger = true, onClick = { confirmDelete = true }) }
        item { Spacer(Modifier.padding(24.dp)) }
    }
    if (confirmOut) Confirm("Unlink this phone?", "It stops reaching your hub until you link it again. Saved chats stay readable here.", "Unlink", onSignOut) { confirmOut = false }
    if (confirmDelete) Confirm("Delete saved chats?", "They are removed from this phone. The hub keeps its history.", "Delete", model::deleteSaved) { confirmDelete = false }
}
