package io.github.phantomconartist.mace.ui

import androidx.compose.foundation.Image
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
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.Logout
import androidx.compose.material.icons.rounded.Edit
import androidx.compose.material.icons.rounded.MoreVert
import androidx.compose.material3.Badge
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.phantomconartist.mace.Conversation
import io.github.phantomconartist.mace.Filter
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.R
import io.github.phantomconartist.mace.Screen
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

@Composable
fun SignInScreen(state: Screen.SignIn, onSignIn: (String) -> Unit) {
    var link by rememberSaveable { mutableStateOf("") }
    Box(Modifier.fillMaxSize().safeDrawingPadding().imePadding().padding(horizontal = 24.dp), contentAlignment = Alignment.Center) {
        Column(Modifier.widthIn(max = 440.dp).fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(16.dp)) {
            Image(painterResource(R.drawable.ic_launcher_foreground), contentDescription = null, modifier = Modifier.size(88.dp))
            Text("Connect to your hub", style = MaterialTheme.typography.headlineMedium, modifier = Modifier.semantics { heading() })
            Text(
                "On your computer, open MACE, then Settings, Phone, and copy the sign-in link. Paste it here. It works once and lasts ten minutes.",
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            OutlinedTextField(link, { link = it }, label = { Text("Sign-in link") }, singleLine = true, enabled = !state.busy, modifier = Modifier.fillMaxWidth())
            state.problem?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
            Button({ onSignIn(link) }, enabled = link.isNotBlank() && !state.busy, modifier = Modifier.fillMaxWidth().height(52.dp)) {
                Text(if (state.busy) "Signing in…" else "Sign in")
            }
        }
    }
}

private fun listTime(at: String): String {
    val t = runCatching { OffsetDateTime.parse(at).atZoneSameInstant(ZoneId.systemDefault()) }.getOrNull() ?: return ""
    val day = t.toLocalDate()
    return when {
        day == LocalDate.now() -> t.format(DateTimeFormatter.ofLocalizedTime(FormatStyle.SHORT))
        day == LocalDate.now().minusDays(1) -> "Yesterday"
        day.isAfter(LocalDate.now().minusDays(7)) -> t.dayOfWeek.getDisplayName(java.time.format.TextStyle.SHORT, java.util.Locale.getDefault())
        else -> day.format(DateTimeFormatter.ofLocalizedDate(FormatStyle.SHORT))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    state: Screen.Home,
    agents: List<Member>,
    onFilter: (Filter) -> Unit,
    onRefresh: () -> Unit,
    onOpen: (String) -> Unit,
    onSignOut: () -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    var picking by remember { mutableStateOf(false) }
    val shown = state.conversations.filter {
        when (state.filter) {
            Filter.All -> true
            Filter.Unread -> it.unread > 0
            Filter.Groups -> it.group
            Filter.Agents -> !it.group
        }
    }
    Scaffold(
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text("MACE", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
                        if (state.hub.isNotEmpty()) Text(state.hub, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                },
                actions = {
                    IconButton({ menu = true }) { Icon(Icons.Rounded.MoreVert, "More") }
                    DropdownMenu(menu, { menu = false }) {
                        DropdownMenuItem({ Text("Sign out") }, leadingIcon = { Icon(Icons.AutoMirrored.Rounded.Logout, null) }, onClick = { menu = false; onSignOut() })
                    }
                },
            )
        },
        floatingActionButton = {
            if (agents.isNotEmpty()) FloatingActionButton({ picking = true }) { Icon(Icons.Rounded.Edit, "New chat") }
        },
    ) { pad ->
        PullToRefreshBox(state.loading, onRefresh, Modifier.padding(pad).fillMaxSize()) {
            LazyColumn(Modifier.fillMaxSize(), contentPadding = PaddingValues(bottom = 88.dp)) {
                // Filter pills, as in the research's inbox: one tap narrows a long list.
                item {
                    LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 4.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        items(Filter.entries) { f ->
                            val count = if (f == Filter.Unread) state.conversations.count { it.unread > 0 } else 0
                            FilterChip(selected = state.filter == f, onClick = { onFilter(f) }, label = { Text(if (count > 0) "${f.label} $count" else f.label) })
                        }
                    }
                }
                state.problem?.let { item { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) } }
                if (!state.loading && state.problem == null && shown.isEmpty()) {
                    item {
                        Text(
                            if (state.conversations.isEmpty()) "No conversations yet. Start an agent on your computer, then tap the pencil to chat with it."
                            else "Nothing here.",
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.padding(24.dp),
                        )
                    }
                }
                items(shown, key = { it.id }) { ConversationRow(it) { onOpen(it.id) } }
            }
        }
    }
    if (picking) {
        ModalBottomSheet({ picking = false }) {
            Text("New chat", style = MaterialTheme.typography.titleMedium, modifier = Modifier.padding(horizontal = 24.dp, vertical = 8.dp))
            LazyColumn(Modifier.navigationBarsPadding()) {
                items(agents, key = { it.id }) { a ->
                    ListItem(
                        headlineContent = { Text(a.name) },
                        supportingContent = { Text(listOfNotNull("@${a.handle}".takeIf { a.handle.isNotEmpty() }, a.machine ?: "this hub").joinToString(" · ")) },
                        leadingContent = { Avatar(a.name, a.id, false, 40) },
                        modifier = Modifier.clickable { picking = false; onOpen(a.id) },
                    )
                }
            }
        }
    }
}

@Composable
private fun ConversationRow(c: Conversation, onClick: () -> Unit) {
    ListItem(
        modifier = Modifier.clickable(onClick = onClick),
        leadingContent = { Avatar(c.name, c.id, c.group, 52) },
        headlineContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(c.name, Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis,
                    fontWeight = if (c.unread > 0) FontWeight.SemiBold else FontWeight.Normal)
                Spacer(Modifier.width(8.dp))
                Text(listTime(c.at), style = MaterialTheme.typography.labelMedium,
                    color = if (c.unread > 0) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant)
            }
        },
        supportingContent = {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    if (c.working) "working…" else c.preview,
                    Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis,
                    color = if (c.working) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (c.unread > 0) {
                    Spacer(Modifier.width(8.dp))
                    Badge(containerColor = MaterialTheme.colorScheme.primary) { Text(c.unread.toString()) }
                }
            }
        },
    )
}
