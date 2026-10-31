package io.github.phantomconartist.mace.ui

import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.ArrowForward
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.GroupAdd
import androidx.compose.material.icons.rounded.SmartToy
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Entry
import io.github.phantomconartist.mace.data.Peer
import io.github.phantomconartist.mace.data.Provider
import io.github.phantomconartist.mace.ui.theme.mace

/** A top bar with a back arrow, a title, and an optional line under it. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PageBar(title: String, subtitle: String? = null, onBack: () -> Unit, actions: @Composable () -> Unit = {}) {
    TopAppBar(
        colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
        navigationIcon = { IconButton(onBack) { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Back") } },
        title = {
            Column {
                Text(title, fontSize = 19.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                subtitle?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant) }
            }
        },
        actions = { actions() },
    )
}

/** A row that starts something: a red circle with an icon, and a label. */
@Composable
fun ActionRow(icon: androidx.compose.ui.graphics.vector.ImageVector, label: String, onClick: () -> Unit) {
    Row(Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 16.dp, vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(48.dp).clip(CircleShape).background(mace.signal), contentAlignment = Alignment.Center) { Icon(icon, null, tint = mace.onSignal) }
        Spacer(Modifier.width(16.dp))
        Text(label, fontSize = 17.sp)
    }
}

@Composable
fun NewChatPage(agents: List<Member>, model: AppModel, onBack: () -> Unit, onNewGroup: () -> Unit, onStartAgent: () -> Unit, onOpen: (String) -> Unit) {
    BackHandler(onBack = onBack)
    var query by remember { mutableStateOf("") }
    val q = query.trim().lowercase()
    Scaffold(containerColor = MaterialTheme.colorScheme.surface, topBar = { PageBar("Select agent", "${agents.size} agents", onBack) }) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize()) {
            item { SearchPill(query, { query = it }, "Search name or @mention") }
            if (q.isEmpty()) {
                item { ActionRow(Icons.Rounded.GroupAdd, "New group", onNewGroup) }
                item { ActionRow(Icons.Rounded.SmartToy, "Start an agent", onStartAgent) }
                item { SectionLabel("Agents on MACE") }
            }
            items(agents.filter { q.isEmpty() || it.name.lowercase().contains(q) || it.handle.lowercase().contains(q) }, key = { it.id }) { a ->
                AgentRow(a, model.media) { onOpen(a.id) }
            }
        }
    }
}

@Composable
fun NewGroupPage(agents: List<Member>, model: AppModel, onBack: () -> Unit, onCreate: (String, List<String>) -> Unit) {
    BackHandler(onBack = onBack)
    var picked by rememberSaveable { mutableStateOf(listOf<String>()) }
    var naming by rememberSaveable { mutableStateOf(false) }
    var name by rememberSaveable { mutableStateOf("") }
    val chosen = picked.mapNotNull { id -> agents.firstOrNull { it.id == id } }
    Scaffold(
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = {
            PageBar(if (naming) "New group" else "Add agents", if (naming) "Name it after the work" else if (picked.isEmpty()) "Choose who works together" else "${picked.size} of ${agents.size} selected",
                onBack = { if (naming) naming = false else onBack() })
        },
        floatingActionButton = {
            if (picked.isNotEmpty() && (!naming || name.isNotBlank())) {
                FloatingActionButton({ if (naming) onCreate(name, picked) else naming = true }, containerColor = mace.signal, contentColor = mace.onSignal, shape = RoundedCornerShape(16.dp)) {
                    Icon(if (naming) Icons.Rounded.Check else Icons.AutoMirrored.Rounded.ArrowForward, if (naming) "Create group" else "Next")
                }
            }
        },
    ) { pad ->
        Column(Modifier.padding(pad).fillMaxSize()) {
            if (naming) {
                Row(Modifier.padding(20.dp), verticalAlignment = Alignment.CenterVertically) {
                    Avatar(name.ifBlank { "G" }, "new-group", true, 56)
                    Spacer(Modifier.width(16.dp))
                    OutlinedTextField(name, { name = it.take(80) }, label = { Text("Group name") }, placeholder = { Text("What are we working on?") }, singleLine = true, modifier = Modifier.weight(1f))
                }
                Text("Agents: ${chosen.joinToString(", ") { it.name }}", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 20.dp))
                return@Column
            }
            if (chosen.isNotEmpty()) {
                LazyRow(contentPadding = PaddingValues(horizontal = 12.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    items(chosen, key = { it.id }) { a ->
                        Column(Modifier.width(64.dp).clickable { picked = picked - a.id }, horizontalAlignment = Alignment.CenterHorizontally) {
                            Box {
                                Avatar(a.name, a.id, false, 52, model.media.avatarUrl(a), model.media)
                                Box(Modifier.align(Alignment.BottomEnd).size(20.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surfaceContainerHighest), contentAlignment = Alignment.Center) {
                                    Icon(Icons.Rounded.Close, "Remove ${a.name}", Modifier.size(14.dp))
                                }
                            }
                            Text(a.name, style = MaterialTheme.typography.labelMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        }
                    }
                }
                HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
            }
            LazyColumn(Modifier.fillMaxSize()) {
                items(agents, key = { it.id }) { a ->
                    AgentRow(a, model.media, trailing = {
                        val on = a.id in picked
                        Box(Modifier.size(24.dp).clip(CircleShape).then(if (on) Modifier.background(mace.signal) else Modifier.border(2.dp, MaterialTheme.colorScheme.outline, CircleShape)), contentAlignment = Alignment.Center) {
                            if (on) Icon(Icons.Rounded.Check, null, tint = mace.onSignal, modifier = Modifier.size(16.dp))
                        }
                    }) { picked = if (a.id in picked) picked - a.id else picked + a.id }
                }
            }
        }
    }
}

/** Starts an agent: which machine, which CLI, and which folder it works in (/ by default). */
@Composable
fun StartAgentPage(model: AppModel, onBack: () -> Unit, onStart: (String?, String, String) -> Unit) {
    BackHandler(onBack = onBack)
    var peers by remember { mutableStateOf<List<Peer>>(emptyList()) }
    var machine by rememberSaveable { mutableStateOf<String?>(null) }
    var providers by remember { mutableStateOf<List<Provider>?>(null) }
    var provider by rememberSaveable { mutableStateOf<String?>(null) }
    var path by rememberSaveable { mutableStateOf(".") }
    var folders by remember { mutableStateOf<List<Entry>?>(null) }
    var problem by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(Unit) { peers = runCatching { model.peers() }.getOrDefault(emptyList()) }
    LaunchedEffect(machine) {
        providers = null; problem = null
        runCatching { model.catalog(machine) }.onSuccess { list -> providers = list; provider = list.firstOrNull { it.available }?.id }.onFailure { problem = it.message; providers = emptyList() }
        path = "."
    }
    LaunchedEffect(machine, path) {
        folders = null
        folders = runCatching { model.machineFiles(machine, path) }.getOrDefault(emptyList()).filter { it.directory && !it.name.startsWith(".") }.sortedBy { it.name.lowercase() }
    }
    val absolute = "/" + if (path == ".") "" else path.trim('/')
    Scaffold(
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = { PageBar("Start an agent", "It works in the folder you choose", onBack) },
        bottomBar = {
            Column(Modifier.background(MaterialTheme.colorScheme.surface).navigationBarsPadding().padding(16.dp)) {
                Text(absolute, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Spacer(Modifier.height(8.dp))
                Button(
                    { provider?.let { onStart(machine, it, absolute) } },
                    enabled = provider != null,
                    colors = ButtonDefaults.buttonColors(containerColor = mace.signal, contentColor = mace.onSignal),
                    modifier = Modifier.fillMaxWidth().height(52.dp),
                ) { Text("Start agent here") }
            }
        },
    ) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize()) {
            item { SectionLabel("Machine") }
            item {
                LazyRow(contentPadding = PaddingValues(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    item { Choice("This hub", machine == null) { machine = null } }
                    items(peers, key = { it.id }) { p -> Choice(p.name.ifEmpty { p.id }, machine == p.id) { machine = p.id } }
                }
            }
            item { SectionLabel("Agent") }
            problem?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(horizontal = 20.dp)) } }
            if (providers == null) item { Text("Looking for agent CLIs…", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 20.dp)) }
            items(providers.orEmpty(), key = { it.id }) { p ->
                Row(
                    Modifier.fillMaxWidth().clickable(enabled = p.available) { provider = p.id }.padding(horizontal = 16.dp, vertical = 10.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Avatar(p.name, p.id, false, 44)
                    Spacer(Modifier.width(16.dp))
                    Column(Modifier.weight(1f)) {
                        Text(p.name.ifEmpty { p.id }, fontSize = 17.sp, color = if (p.available) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurfaceVariant)
                        Text(if (p.available) p.id else "Not installed on this machine", style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    androidx.compose.material3.RadioButton(provider == p.id, { provider = p.id }, enabled = p.available,
                        colors = androidx.compose.material3.RadioButtonDefaults.colors(selectedColor = mace.signal))
                }
            }
            item { SectionLabel("Folder") }
            if (path != ".") item {
                Row(Modifier.fillMaxWidth().clickable { path = path.trimEnd('/').substringBeforeLast('/', ".") }.padding(horizontal = 24.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.AutoMirrored.Rounded.ArrowBack, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.width(24.dp))
                    Text("Up one folder", fontSize = 16.sp)
                }
            }
            if (folders == null) item { Text("Opening…", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp)) }
            if (folders?.isEmpty() == true) item { Text("No folders inside.", color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp)) }
            items(folders.orEmpty(), key = { it.path }) { f ->
                Row(Modifier.fillMaxWidth().clickable { path = f.path }.padding(horizontal = 24.dp, vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                    Icon(Icons.Rounded.Folder, null, tint = mace.names[1])
                    Spacer(Modifier.width(24.dp))
                    Text(f.name, fontSize = 16.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}

@Composable
fun Choice(label: String, selected: Boolean, onClick: () -> Unit) {
    Row(
        Modifier.height(36.dp).clip(RoundedCornerShape(18.dp))
            .then(if (selected) Modifier.background(MaterialTheme.colorScheme.primaryContainer) else Modifier.border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(18.dp)))
            .clickable(onClick = onClick).padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        if (selected) { Icon(Icons.Rounded.Check, null, Modifier.size(16.dp), tint = MaterialTheme.colorScheme.onPrimaryContainer); Spacer(Modifier.width(6.dp)) }
        Text(label, color = if (selected) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
