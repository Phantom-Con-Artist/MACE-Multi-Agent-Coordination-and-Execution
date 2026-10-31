package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Chat
import androidx.compose.material.icons.automirrored.rounded.Logout
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.AddComment
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Computer
import androidx.compose.material.icons.rounded.Delete
import androidx.compose.material.icons.rounded.DoneAll
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.GroupAdd
import androidx.compose.material.icons.rounded.Groups
import androidx.compose.material.icons.rounded.Info
import androidx.compose.material.icons.rounded.LinkOff
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material.icons.rounded.Search
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material.icons.rounded.SmartToy
import androidx.compose.material.icons.rounded.Tune
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import io.github.phantomconartist.mace.Conversation
import io.github.phantomconartist.mace.Filter
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.R
import io.github.phantomconartist.mace.Screen
import io.github.phantomconartist.mace.ui.theme.mace

enum class Tab(val label: String) { Chats("Chats"), Agents("Agents"), Machines("Machines"), Settings("Settings") }

/** What the home screen asks of the app. */
class HomeActions(
    val onFilter: (Filter) -> Unit,
    val onRefresh: () -> Unit,
    val onOpen: (String) -> Unit,
    val onInfo: (String) -> Unit,
    val onNewChat: () -> Unit,
    val onNewGroup: () -> Unit,
    val onStartAgent: () -> Unit,
    val onRemove: (String) -> Unit,
    val onMarkRead: (Set<String>) -> Unit,
    val onSignOut: () -> Unit,
    val onLinked: () -> Unit,
    val onLink: () -> Unit,
)

/** Shown on saved chats while the phone is not linked to a hub. */
@Composable
fun LinkBar(onLink: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp).clip(RoundedCornerShape(16.dp))
            .background(MaterialTheme.colorScheme.primaryContainer).clickable(onClick = onLink).padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Rounded.LinkOff, null, tint = MaterialTheme.colorScheme.onPrimaryContainer)
        Spacer(Modifier.width(14.dp))
        Column(Modifier.weight(1f)) {
            Text("Not linked to a hub", fontSize = 16.sp, color = MaterialTheme.colorScheme.onPrimaryContainer)
            Text("Showing saved chats. Link to send and see new messages.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onPrimaryContainer.copy(alpha = 0.8f))
        }
        Spacer(Modifier.width(8.dp))
        Text("Link", fontSize = 15.sp, color = mace.onSignal, modifier = Modifier.clip(RoundedCornerShape(16.dp)).background(mace.signal).padding(horizontal = 14.dp, vertical = 6.dp))
    }
}

/** The pill search field at the top of the chat list, as in a messenger. */
@Composable
fun SearchPill(query: String, onQuery: (String) -> Unit, hint: String) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 6.dp).height(50.dp).clip(RoundedCornerShape(25.dp))
            .background(MaterialTheme.colorScheme.surfaceContainerHigh).padding(horizontal = 16.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(Icons.Rounded.Search, null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
        Spacer(Modifier.width(14.dp))
        Box(Modifier.weight(1f)) {
            if (query.isEmpty()) Text(hint, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
            BasicTextField(query, onQuery, singleLine = true, textStyle = MaterialTheme.typography.bodyLarge.copy(color = MaterialTheme.colorScheme.onSurface),
                cursorBrush = SolidColor(MaterialTheme.colorScheme.primary), modifier = Modifier.fillMaxWidth())
        }
        if (query.isNotEmpty()) IconButton({ onQuery("") }, Modifier.size(32.dp)) { Icon(Icons.Rounded.Close, "Clear search", Modifier.size(20.dp)) }
    }
}

@Composable
private fun Pill(label: String, count: Int, selected: Boolean, onClick: () -> Unit) {
    Row(
        Modifier.height(36.dp).clip(RoundedCornerShape(18.dp))
            .then(if (selected) Modifier.background(MaterialTheme.colorScheme.primaryContainer) else Modifier.border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(18.dp)))
            .clickable(onClick = onClick).padding(horizontal = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, style = MaterialTheme.typography.bodyLarge, fontSize = 15.sp,
            color = if (selected) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurfaceVariant)
        if (count > 0) {
            Spacer(Modifier.width(6.dp))
            Text(count.toString(), fontSize = 13.sp, color = if (selected) MaterialTheme.colorScheme.onPrimaryContainer else MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.8f))
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    state: Screen.Home,
    agents: List<Member>,
    model: io.github.phantomconartist.mace.AppModel,
    actions: HomeActions,
) {
    var tab by rememberSaveable { mutableStateOf(Tab.Chats) }
    var menu by remember { mutableStateOf(false) }
    var query by rememberSaveable { mutableStateOf("") }
    var selected by remember { mutableStateOf(setOf<String>()) }
    var confirm by remember { mutableStateOf(false) }
    var peek by remember { mutableStateOf<Conversation?>(null) }
    val unreadChats = state.conversations.count { it.unread > 0 }

    Scaffold(
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = {
            if (tab == Tab.Chats && selected.isNotEmpty()) {
                TopAppBar(
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
                    navigationIcon = { IconButton({ selected = emptySet() }) { Icon(Icons.Rounded.Close, "Cancel") } },
                    title = { Text(selected.size.toString()) },
                    actions = {
                        IconButton({ actions.onMarkRead(selected); selected = emptySet() }) { Icon(Icons.Rounded.DoneAll, "Mark as read") }
                        IconButton({ confirm = true }) { Icon(Icons.Rounded.Delete, "Remove") }
                    },
                )
            } else {
                TopAppBar(
                    colors = TopAppBarDefaults.topAppBarColors(containerColor = MaterialTheme.colorScheme.surface),
                    title = {
                        Text(if (tab == Tab.Chats) "MACE" else tab.label, fontSize = 26.sp, fontWeight = FontWeight.Bold,
                            color = if (tab == Tab.Chats) MaterialTheme.colorScheme.onSurface else MaterialTheme.colorScheme.onSurface)
                    },
                    actions = {
                        if (tab == Tab.Chats) IconButton(actions.onNewGroup) { Icon(Icons.Rounded.GroupAdd, "New group") }
                        if (tab == Tab.Agents) IconButton(actions.onStartAgent) { Icon(Icons.Rounded.Add, "Start an agent") }
                        IconButton({ menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                        DropdownMenu(menu, { menu = false }) {
                            DropdownMenuItem({ Text("New group") }, onClick = { menu = false; actions.onNewGroup() })
                            DropdownMenuItem({ Text("Start an agent") }, onClick = { menu = false; actions.onStartAgent() })
                            DropdownMenuItem({ Text("Settings") }, onClick = { menu = false; tab = Tab.Settings })
                            DropdownMenuItem({ Text("Sign out") }, leadingIcon = { Icon(Icons.AutoMirrored.Rounded.Logout, null) }, onClick = { menu = false; actions.onSignOut() })
                        }
                    },
                )
            }
        },
        floatingActionButton = {
            when (tab) {
                Tab.Chats -> FloatingActionButton(actions.onNewChat, containerColor = mace.signal, contentColor = mace.onSignal, shape = RoundedCornerShape(16.dp)) {
                    Icon(Icons.Rounded.AddComment, "New chat")
                }
                Tab.Agents -> FloatingActionButton(actions.onStartAgent, containerColor = mace.signal, contentColor = mace.onSignal, shape = RoundedCornerShape(16.dp)) {
                    Icon(Icons.Rounded.Add, "Start an agent")
                }
                else -> {}
            }
        },
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface, tonalElevation = 0.dp) {
                Tab.entries.forEach { t ->
                    NavigationBarItem(
                        selected = tab == t,
                        onClick = { tab = t; selected = emptySet() },
                        icon = {
                            androidx.compose.material3.BadgedBox(badge = {
                                if (t == Tab.Chats && unreadChats > 0) androidx.compose.material3.Badge(containerColor = mace.signal, contentColor = mace.onSignal) { Text(unreadChats.toString()) }
                            }) {
                                Icon(when (t) { Tab.Chats -> Icons.AutoMirrored.Rounded.Chat; Tab.Agents -> Icons.Rounded.SmartToy; Tab.Machines -> Icons.Rounded.Computer; Tab.Settings -> Icons.Rounded.Settings }, null)
                            }
                        },
                        label = { Text(t.label, fontWeight = if (tab == t) FontWeight.Bold else FontWeight.Normal) },
                        colors = NavigationBarItemDefaults.colors(
                            indicatorColor = MaterialTheme.colorScheme.primaryContainer,
                            selectedIconColor = MaterialTheme.colorScheme.onPrimaryContainer,
                            selectedTextColor = MaterialTheme.colorScheme.onSurface,
                            unselectedIconColor = MaterialTheme.colorScheme.onSurface,
                            unselectedTextColor = MaterialTheme.colorScheme.onSurface,
                        ),
                    )
                }
            }
        },
    ) { pad ->
        Box(Modifier.padding(pad).fillMaxSize()) {
            when (tab) {
                Tab.Chats -> ChatsTab(state, query, { query = it }, selected, { selected = it }, model, actions, onPeek = { peek = it })
                Tab.Agents -> AgentsTab(agents, model, actions)
                Tab.Machines -> MachinesTab(model)
                Tab.Settings -> SettingsTab(state.hub, model, actions.onSignOut, actions.onLinked, actions.onLink)
            }
        }
    }
    if (confirm) {
        val groups = selected.count { id -> state.conversations.any { it.id == id && it.group } }
        Confirm(
            title = if (selected.size == 1) "Remove this chat?" else "Remove ${selected.size} chats?",
            text = buildString {
                if (groups > 0) append("Removing a group stops its work. ")
                if (groups < selected.size) append("Removing an agent's chat removes the agent from every chat and group. ")
                append("History stays on the hub.")
            },
            action = "Remove",
            onConfirm = { selected.forEach(actions.onRemove); selected = emptySet() },
            onDismiss = { confirm = false },
        )
    }
    peek?.let { c -> QuickView(c, model.media, onDismiss = { peek = null }, onOpen = { peek = null; actions.onOpen(c.id) }, onInfo = { peek = null; actions.onInfo(c.id) }) }
}

@Composable
private fun ChatsTab(
    state: Screen.Home,
    query: String,
    onQuery: (String) -> Unit,
    selected: Set<String>,
    onSelect: (Set<String>) -> Unit,
    model: io.github.phantomconartist.mace.AppModel,
    actions: HomeActions,
    onPeek: (Conversation) -> Unit,
) {
    val linked by model.linked.collectAsState()
    val q = query.trim().lowercase()
    val shown = state.conversations.filter {
        when (state.filter) {
            Filter.All -> true
            Filter.Unread -> it.unread > 0
            Filter.Groups -> it.group
            Filter.Agents -> !it.group
        } && (q.isEmpty() || it.name.lowercase().contains(q) || it.preview.lowercase().contains(q))
    }
    PullToRefreshBox(state.loading, actions.onRefresh, Modifier.fillMaxSize()) {
        LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 96.dp)) {
            if (!linked) item { LinkBar(actions.onLink) }
            item { SearchPill(query, onQuery, "Search chats") }
            item {
                LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    items(Filter.entries) { f ->
                        val count = when (f) {
                            Filter.Unread -> state.conversations.count { it.unread > 0 }
                            Filter.Groups -> state.conversations.count { it.group }
                            else -> 0
                        }
                        Pill(f.label, count, state.filter == f) { actions.onFilter(f) }
                    }
                    item {
                        Box(Modifier.size(36.dp).clip(CircleShape).border(1.dp, MaterialTheme.colorScheme.outlineVariant, CircleShape).clickable(onClick = actions.onNewGroup), contentAlignment = Alignment.Center) {
                            Icon(Icons.Rounded.Add, "New group", tint = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.size(20.dp))
                        }
                    }
                }
            }
            state.problem?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) } }
            if (!state.loading && state.problem == null && shown.isEmpty()) {
                item {
                    Text(
                        when {
                            q.isNotEmpty() -> "No chats match \"$query\"."
                            state.conversations.isEmpty() -> "No conversations yet. Tap the new-chat button to talk to an agent, or start one."
                            else -> "Nothing here."
                        },
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(24.dp),
                    )
                }
            }
            items(shown, key = { it.id }) { c ->
                ConversationRow(
                    c, model.media, c.id in selected,
                    onClick = { if (selected.isNotEmpty()) onSelect(if (c.id in selected) selected - c.id else selected + c.id) else actions.onOpen(c.id) },
                    onLongClick = { onSelect(selected + c.id) },
                    onAvatar = { if (selected.isEmpty()) onPeek(c) else onSelect(if (c.id in selected) selected - c.id else selected + c.id) },
                )
            }
        }
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
private fun ConversationRow(c: Conversation, media: MediaLoader, selected: Boolean, onClick: () -> Unit, onLongClick: () -> Unit, onAvatar: () -> Unit) {
    Row(
        Modifier.fillMaxWidth()
            .background(if (selected) MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.55f) else androidx.compose.ui.graphics.Color.Transparent)
            .combinedClickable(onClick = onClick, onLongClick = onLongClick)
            .padding(horizontal = 16.dp, vertical = 11.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        val asleep = c.online == 0
        Box(Modifier.clickable(onClick = onAvatar)) {
            Box(Modifier.clip(CircleShape)) { Avatar(c.name, c.id, c.group, 54, c.photo, media) }
            // Green: working now. Red: down (machine offline or agent stopped). No dot: online and idle.
            if (!selected && (c.working || asleep)) {
                Box(Modifier.align(Alignment.BottomEnd).size(16.dp).clip(CircleShape).background(MaterialTheme.colorScheme.surface).padding(2.5.dp).clip(CircleShape)
                    .background(if (c.working) mace.online else mace.down))
            }
            if (selected) {
                Box(Modifier.align(Alignment.BottomEnd).size(22.dp).clip(CircleShape).background(mace.signal).border(2.dp, MaterialTheme.colorScheme.surface, CircleShape), contentAlignment = Alignment.Center) {
                    Icon(Icons.Rounded.Check, null, tint = mace.onSignal, modifier = Modifier.size(14.dp))
                }
            }
        }
        Spacer(Modifier.width(16.dp))
        Column(Modifier.weight(1f)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(c.name, Modifier.weight(1f), fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurface,
                    fontWeight = FontWeight.Medium)
                Spacer(Modifier.width(8.dp))
                Text(listTime(c.at), fontSize = 12.sp,
                    color = if (c.unread > 0) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.height(3.dp))
            Row(verticalAlignment = Alignment.CenterVertically) {
                if (asleep && !c.working) {
                    Text("Offline", fontSize = 13.sp, color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(end = 6.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceContainerHigh).padding(horizontal = 5.dp, vertical = 1.dp))
                }
                Text(
                    if (c.working) "working…" else c.preview,
                    Modifier.weight(1f), fontSize = 15.sp, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    color = if (c.working) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (c.unread > 0) {
                    Spacer(Modifier.width(8.dp))
                    Box(Modifier.height(22.dp).widthIn(min = 22.dp).clip(CircleShape).background(mace.signal).padding(horizontal = 6.dp), contentAlignment = Alignment.Center) {
                        Text(if (c.unread > 99) "99+" else c.unread.toString(), fontSize = 12.sp, fontWeight = FontWeight.Bold, color = mace.onSignal)
                    }
                }
            }
        }
    }
}

/** Tapping an avatar in the list: the photo, large, with quick actions, as in a messenger. */
@Composable
fun QuickView(c: Conversation, media: MediaLoader, onDismiss: () -> Unit, onOpen: () -> Unit, onInfo: () -> Unit) {
    Dialog(onDismissRequest = onDismiss) {
        Column(Modifier.width(280.dp).clip(RoundedCornerShape(4.dp)).background(MaterialTheme.colorScheme.surfaceContainer)) {
            Box(Modifier.size(280.dp).background(memberColor(c.id).copy(alpha = 0.2f)), contentAlignment = Alignment.Center) {
                val img = rememberImage(media, c.photo, 840)
                if (img != null) Image(img, c.name, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize())
                else if (c.group) Icon(Icons.Rounded.Groups, null, tint = memberColor(c.id), modifier = Modifier.size(140.dp))
                else Text(c.name.trim().take(1).uppercase(), color = memberColor(c.id), fontSize = 120.sp, fontWeight = FontWeight.SemiBold)
                Text(c.name, fontSize = 18.sp, color = androidx.compose.ui.graphics.Color.White, maxLines = 1, overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.align(Alignment.TopStart).fillMaxWidth().background(androidx.compose.ui.graphics.Color(0x59000000)).padding(horizontal = 14.dp, vertical = 10.dp))
            }
            Row(Modifier.fillMaxWidth().padding(vertical = 4.dp), horizontalArrangement = Arrangement.SpaceEvenly) {
                IconButton(onOpen) { Icon(Icons.AutoMirrored.Rounded.Chat, "Message", tint = MaterialTheme.colorScheme.primary) }
                IconButton(onInfo) { Icon(if (c.group) Icons.Rounded.Tune else Icons.Rounded.Folder, if (c.group) "Conversation mode" else "Files", tint = MaterialTheme.colorScheme.primary) }
                IconButton(onInfo) { Icon(Icons.Rounded.Info, "Info", tint = MaterialTheme.colorScheme.primary) }
            }
        }
    }
}
