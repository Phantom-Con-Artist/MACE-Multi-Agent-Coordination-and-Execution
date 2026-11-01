package io.github.phantomconartist.mace.ui

import androidx.activity.compose.BackHandler
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
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material.icons.rounded.DeleteOutline
import androidx.compose.material.icons.rounded.Inventory2
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExtendedFloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.AppModel
import io.github.phantomconartist.mace.data.Workspace
import io.github.phantomconartist.mace.ui.theme.mace

// Sessions: a new session starts with no agents, groups, or history on every
// linked machine; earlier ones stay saved, each on its own, until removed.

@Composable
fun SessionsPage(model: AppModel, onBack: () -> Unit) {
    BackHandler(onBack = onBack)
    val list by model.workspaces.collectAsState()
    var naming by remember { mutableStateOf(false) }
    var switching by remember { mutableStateOf<Workspace?>(null) }
    var removing by remember { mutableStateOf<Workspace?>(null) }
    Scaffold(
        containerColor = MaterialTheme.colorScheme.surface,
        topBar = { PageBar("Sessions", onBack = onBack) },
        floatingActionButton = {
            ExtendedFloatingActionButton(
                onClick = { naming = true },
                containerColor = mace.signal, contentColor = mace.onSignal, shape = RoundedCornerShape(16.dp),
                icon = { Icon(Icons.Rounded.Add, null) }, text = { Text("New session") },
            )
        },
    ) { pad ->
        LazyColumn(Modifier.padding(pad).fillMaxSize(), contentPadding = PaddingValues(bottom = 96.dp)) {
            item {
                Text(
                    "Each session has its own agents, groups, chats, statuses, and history, on this hub and every linked machine. A new session starts empty; earlier ones stay saved until you remove them. Linked machines and signed-in devices stay the same.",
                    style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(horizontal = 24.dp, vertical = 12.dp),
                )
            }
            items(list.sortedByDescending { it.active.ifEmpty { it.created } }, key = { it.id }) { w ->
                Row(
                    Modifier.fillMaxWidth().clickable(enabled = !w.current) { switching = w }.padding(horizontal = 16.dp, vertical = 12.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Box(Modifier.size(48.dp).clip(CircleShape).background(if (w.current) mace.signal else MaterialTheme.colorScheme.surfaceContainerHigh), contentAlignment = Alignment.Center) {
                        Icon(if (w.current) Icons.Rounded.Check else Icons.Rounded.Inventory2, null, tint = if (w.current) mace.onSignal else MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    Spacer(Modifier.width(16.dp))
                    Column(Modifier.weight(1f)) {
                        Text(w.name, fontSize = 17.sp, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(if (w.current) "Running now" else "Saved · started " + listTime(w.created),
                            style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    }
                    if (!w.current) IconButton({ removing = w }) { Icon(Icons.Rounded.DeleteOutline, "Remove ${w.name}", tint = MaterialTheme.colorScheme.onSurfaceVariant) }
                }
            }
        }
    }
    if (naming) {
        var name by remember { mutableStateOf("") }
        AlertDialog(
            onDismissRequest = { naming = false },
            title = { Text("New session") },
            text = {
                Column {
                    Text("Every agent stops, and this hub and your linked machines start again with nothing in them. The current session is saved.",
                        style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    Spacer(Modifier.size(12.dp))
                    OutlinedTextField(name, { name = it }, label = { Text("Name") }, placeholder = { Text("Launch site") }, singleLine = true, modifier = Modifier.fillMaxWidth())
                }
            },
            confirmButton = { TextButton({ naming = false; model.newSession(name.trim()); onBack() }) { Text("Start") } },
            dismissButton = { TextButton({ naming = false }) { Text("Cancel") } },
        )
    }
    switching?.let { w ->
        Confirm("Switch to ${w.name}?", "Every agent stops, and this hub and your linked machines reopen ${w.name} as it was saved. The current session is saved.", "Switch",
            { model.switchSession(w.id, w.name); onBack() }) { switching = null }
    }
    removing?.let { w ->
        Confirm("Remove ${w.name}?", "Its agents, groups, chats, and shared files on this hub are deleted. This cannot be undone.", "Remove",
            { model.removeSession(w.id) }) { removing = null }
    }
}
