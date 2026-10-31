package io.github.phantomconartist.mace.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.rounded.AlternateEmail
import androidx.compose.material.icons.rounded.AttachFile
import androidx.compose.material.icons.rounded.FolderOpen
import androidx.compose.material.icons.rounded.PhotoLibrary
import androidx.compose.material.icons.rounded.Description
import androidx.compose.material.icons.rounded.Groups
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.graphicsLayer
import io.github.phantomconartist.mace.ui.theme.mace
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.ArrowBack
import androidx.compose.material.icons.automirrored.rounded.Send
import androidx.compose.material.icons.rounded.Add
import androidx.compose.material.icons.rounded.Close
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.FilledTonalButton
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.InputChip
import androidx.compose.material3.ListItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextField
import androidx.compose.material3.TextFieldDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.input.TextFieldValue
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Entry
import io.github.phantomconartist.mace.data.Models

// The composer works like the desktop's: @ opens a menu of coworkers in a
// group, / opens commands in a private chat (then models after /model and
// folders after /cd), and the paperclip attaches a file or folder from the hub.

data class ChatCommand(val name: String, val args: String = "", val detail: String)

/** The same commands as the desktop (web/src/messenger-parts.tsx). */
val chatCommands = listOf(
    ChatCommand("model", "<name>", "Set the model everywhere this agent works"),
    ChatCommand("models", detail = "List models and show the current one"),
    ChatCommand("cd", "<folder>", "Change the working folder"),
    ChatCommand("new", detail = "Start a fresh native session"),
    ChatCommand("stop", detail = "Stop the current turn and clear waiting work"),
    ChatCommand("status", detail = "Model, folder, waiting work, and groups"),
    ChatCommand("rename", "<handle>", "Change the mention handle"),
    ChatCommand("files", detail = "Browse files at the agent's folder"),
    ChatCommand("help", detail = "List commands"),
)

/** A known command at the start of a draft, or null for an ordinary message. */
fun parseCommand(text: String): Pair<String, String>? {
    val m = Regex("""^/([a-z]+)(?:\s+([\s\S]*))?$""").find(text.trim()) ?: return null
    val name = m.groupValues[1]
    return if (chatCommands.any { it.name == name }) name to m.groupValues[2].trim() else null
}

/** One row of the menu above the composer. Picking puts text in the draft; run sends it. */
data class Suggestion(
    val id: String,
    val label: String,
    val detail: String,
    val text: String,
    val run: Boolean = false,
    val mono: Boolean = false,
    /** The agent this row mentions, for its avatar. */
    val member: Member? = null,
    /** Mentions everyone in the group. */
    val everyone: Boolean = false,
)

private val mentionAtCaret = Regex("""(^|\s)@([\w.-]*)$""")

/** Coworkers to mention at the caret, everyone first, as in a messenger group. */
fun mentionSuggestions(draft: TextFieldValue, members: List<Member>): List<Suggestion> {
    val before = draft.text.substring(0, draft.selection.start.coerceIn(0, draft.text.length))
    val m = mentionAtCaret.find(before) ?: return emptyList()
    val q = m.groupValues[2].lowercase()
    val start = before.length - m.groupValues[2].length
    fun next(h: String) = draft.text.substring(0, start) + h + " " + draft.text.substring(before.length)
    val all = Suggestion("@all", "all", "Mention every agent in this chat", next("all"), everyone = true).takeIf { "all".startsWith(q) }
    val people = members.filter { it.handle.isNotEmpty() && (it.handle.lowercase().startsWith(q) || it.name.lowercase().contains(q)) }.map {
        Suggestion("@" + it.handle, it.name, listOfNotNull("@" + it.handle, it.machine, statusWord(it.status)).joinToString(" · "), next(it.handle), member = it)
    }
    return listOfNotNull(all) + people
}

/** An agent's status in plain words. */
fun statusWord(status: String): String? = when (status) {
    "replying" -> "Working"
    "stopped" -> "Stopped"
    "terminated" -> "Terminated"
    "ready", "" -> null
    else -> status.replaceFirstChar { it.uppercase() }
}

/** Where /cd is pointing: the folder to list (absolute) and the partial name typed after it. */
fun cdTarget(typed: String, home: String): Pair<String, String> {
    val dirPart = typed.substringBeforeLast('/', "")
    val partial = typed.substringAfterLast('/')
    val dir = when {
        typed.startsWith("/") -> "/" + dirPart.trimStart('/')
        dirPart.isEmpty() -> home
        else -> home.trimEnd('/') + "/" + dirPart
    }
    return dir to partial
}

fun commandSuggestions(text: String, models: Models?, folders: Pair<String, List<String>>?, home: String): List<Suggestion> {
    Regex("""^/([a-z]*)$""").find(text)?.let { m ->
        return chatCommands.filter { it.name.startsWith(m.groupValues[1]) }.map { c ->
            Suggestion(c.name, "/" + c.name + if (c.args.isNotEmpty()) " " + c.args else "", c.detail, "/" + c.name + if (c.args.isNotEmpty()) " " else "", run = c.args.isEmpty())
        }
    }
    Regex("""^/model\s+(\S*)$""").find(text)?.let { m ->
        val list = models ?: return emptyList()
        val q = m.groupValues[1].lowercase()
        val current = list.current.ifEmpty { "default" }
        return (listOf(io.github.phantomconartist.mace.data.ModelOption("default", "CLI default")) + list.models)
            .filter { it.id.lowercase().contains(q) || it.label.lowercase().contains(q) }
            .map { o -> Suggestion(o.id, o.id, listOf(o.label.takeIf { it != o.id }.orEmpty(), if (o.id == current) "current" else "").filter { it.isNotEmpty() }.joinToString(" · "), "/model ${o.id}", run = true, mono = true) }
    }
    Regex("""^/cd\s+(.*)$""").find(text)?.let { m ->
        val typed = m.groupValues[1]
        val (dir, partial) = cdTarget(typed, home)
        val names = folders?.takeIf { it.first == dir }?.second ?: return emptyList()
        val prefix = typed.substringBeforeLast('/', "").let { if (it.isEmpty() && typed.startsWith("/")) "/" else if (it.isEmpty()) "" else "$it/" }
        val matches = names.filter { it.lowercase().startsWith(partial.lowercase()) }.take(7).map { n ->
            Suggestion(n, "$n/", dir.trimEnd('/') + "/" + n, "/cd $prefix$n/", mono = true)
        }
        return if (partial.isEmpty() && typed.isNotEmpty() && matches.isNotEmpty()) listOf(Suggestion(".", "Use this folder", dir, text, run = true, mono = true)) + matches else matches
    }
    return emptyList()
}

/** The floating card above the composer: mentions, commands, models, folders. */
@Composable
fun SuggestionMenu(options: List<Suggestion>, onPick: (Suggestion) -> Unit) {
    if (options.isEmpty()) return
    Surface(
        color = MaterialTheme.colorScheme.surfaceContainerHigh,
        shape = RoundedCornerShape(18.dp),
        shadowElevation = 6.dp,
        modifier = Modifier.padding(start = 8.dp, end = 64.dp, bottom = 4.dp),
    ) {
        LazyColumn(Modifier.fillMaxWidth().heightIn(max = 300.dp).padding(vertical = 6.dp)) {
            items(options, key = { it.id + it.text }) { o ->
                Row(Modifier.fillMaxWidth().clickable { onPick(o) }.padding(horizontal = 14.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    when {
                        o.member != null -> Avatar(o.member.name, o.member.id, false, 40)
                        o.everyone -> Box(Modifier.size(40.dp), contentAlignment = Alignment.Center) { Icon(Icons.Rounded.Groups, null, tint = MaterialTheme.colorScheme.onSurfaceVariant) }
                    }
                    if (o.member != null || o.everyone) Spacer(Modifier.width(14.dp))
                    Column(Modifier.weight(1f)) {
                        Text(o.label, style = MaterialTheme.typography.bodyLarge, fontFamily = if (o.mono) FontFamily.Monospace else null, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (o.detail.isNotEmpty()) Text(o.detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    }
                }
            }
        }
    }
}

/**
 * The composer, laid out like a messenger's: one rounded field holding the
 * reply quote, the attachment, a leading key (@ in groups, / for an agent's
 * commands), the text, and the paperclip; the round send button beside it.
 */
@Composable
fun Composer(
    draft: TextFieldValue,
    quote: Quote?,
    attachment: String,
    suggestions: List<Suggestion>,
    group: Boolean,
    onDraft: (TextFieldValue) -> Unit,
    onPick: (Suggestion) -> Unit,
    onClearQuote: () -> Unit,
    onAttach: () -> Unit,
    onClearAttachment: () -> Unit,
    onSend: () -> Unit,
) {
    val ready = draft.text.isNotBlank() || attachment.isNotEmpty()
    Column(Modifier.fillMaxWidth().navigationBarsPadding().imePadding()) {
        SuggestionMenu(suggestions, onPick)
        Row(Modifier.fillMaxWidth().padding(start = 6.dp, end = 6.dp, top = 2.dp, bottom = 6.dp), verticalAlignment = Alignment.Bottom) {
            Surface(shape = RoundedCornerShape(26.dp), color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = Modifier.weight(1f)) {
                Column {
                    AnimatedVisibility(quote != null) {
                        quote?.let { q ->
                            Row(
                                Modifier.fillMaxWidth().padding(start = 6.dp, end = 6.dp, top = 6.dp)
                                    .clip(RoundedCornerShape(12.dp)).background(MaterialTheme.colorScheme.surfaceContainerHighest),
                                verticalAlignment = Alignment.CenterVertically,
                            ) {
                                Box(Modifier.width(4.dp).heightIn(min = 52.dp).background(memberColor(q.sender)))
                                Column(Modifier.weight(1f).padding(horizontal = 10.dp, vertical = 6.dp)) {
                                    Text(q.name, style = MaterialTheme.typography.labelLarge, color = memberColor(q.sender))
                                    Text(q.text, style = MaterialTheme.typography.bodySmall, maxLines = 2, overflow = TextOverflow.Ellipsis, color = MaterialTheme.colorScheme.onSurfaceVariant)
                                }
                                IconButton(onClick = onClearQuote) { Icon(Icons.Rounded.Close, "Cancel reply", Modifier.size(20.dp)) }
                            }
                        }
                    }
                    if (attachment.isNotEmpty()) {
                        InputChip(
                            selected = true,
                            onClick = onClearAttachment,
                            label = { Text(attachment.trimEnd('/').substringAfterLast('/') + if (attachment.endsWith("/")) "/" else "", maxLines = 1, overflow = TextOverflow.Ellipsis) },
                            leadingIcon = { Icon(if (attachment.endsWith("/")) Icons.Rounded.Folder else Icons.Rounded.InsertDriveFile, null, Modifier.size(18.dp)) },
                            trailingIcon = { Icon(Icons.Rounded.Close, "Remove attachment", Modifier.size(18.dp)) },
                            modifier = Modifier.padding(start = 12.dp, top = 6.dp),
                        )
                    }
                    Row(verticalAlignment = Alignment.Bottom) {
                        val key = if (group) "@" else "/"
                        IconButton(
                            onClick = {
                                val at = draft.selection.start.coerceIn(0, draft.text.length)
                                val pad = if (at > 0 && !draft.text[at - 1].isWhitespace()) " " else ""
                                val text = if (group) draft.text.substring(0, at) + pad + key + draft.text.substring(at) else key + draft.text.removePrefix("/")
                                onDraft(TextFieldValue(text, TextRange(if (group) at + pad.length + 1 else 1)))
                            },
                            modifier = Modifier.size(48.dp),
                        ) {
                            Icon(if (group) Icons.Rounded.AlternateEmail else Icons.Rounded.Terminal, if (group) "Mention an agent" else "Commands",
                                tint = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        TextField(
                            value = draft,
                            onValueChange = onDraft,
                            placeholder = { Text("Message", style = MaterialTheme.typography.bodyLarge) },
                            textStyle = MaterialTheme.typography.bodyLarge,
                            maxLines = 6,
                            colors = TextFieldDefaults.colors(
                                focusedIndicatorColor = Color.Transparent, unfocusedIndicatorColor = Color.Transparent,
                                focusedContainerColor = Color.Transparent, unfocusedContainerColor = Color.Transparent,
                                cursorColor = MaterialTheme.colorScheme.primary,
                            ),
                            modifier = Modifier.weight(1f).heightIn(min = 48.dp),
                        )
                        IconButton(onClick = onAttach, modifier = Modifier.size(48.dp)) {
                            Icon(Icons.Rounded.AttachFile, "Attach a file from the hub", tint = MaterialTheme.colorScheme.onSurfaceVariant,
                                modifier = Modifier.graphicsLayer { rotationZ = 45f })
                        }
                    }
                }
            }
            Spacer(Modifier.width(6.dp))
            // The send arrow is a triangle: its weight sits left of its box,
            // so it moves right a little to look centered in the circle.
            Box(
                Modifier.size(50.dp).clip(CircleShape).background(mace.signal).clickable(enabled = ready, onClick = onSend),
                contentAlignment = Alignment.Center,
            ) {
                Icon(Icons.AutoMirrored.Rounded.Send, "Send", tint = mace.onSignal.copy(alpha = if (ready) 1f else 0.6f),
                    modifier = Modifier.size(24.dp).offset(x = 2.dp))
            }
        }
    }
}

/** What the paperclip can attach, as a grid like a messenger's attach sheet. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AttachSheet(agentFolder: String?, onPick: (String) -> Unit, onPhone: (Boolean) -> Unit, onDismiss: () -> Unit) {
    ModalBottomSheet(onDismissRequest = onDismiss, containerColor = MaterialTheme.colorScheme.surfaceContainer) {
        @Composable
        fun androidx.compose.foundation.layout.RowScope.Tile(label: String, icon: androidx.compose.ui.graphics.vector.ImageVector, tint: Color, onClick: () -> Unit) {
            Column(Modifier.weight(1f).clip(RoundedCornerShape(12.dp)).clickable(onClick = onClick).padding(vertical = 8.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Box(Modifier.size(width = 64.dp, height = 44.dp).clip(RoundedCornerShape(22.dp)).background(MaterialTheme.colorScheme.surfaceContainerHighest), contentAlignment = Alignment.Center) {
                    Icon(icon, null, tint = tint)
                }
                Spacer(Modifier.height(8.dp))
                Text(label, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1)
            }
        }
        Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 32.dp).navigationBarsPadding()) {
            Tile("Gallery", Icons.Rounded.PhotoLibrary, mace.names[3]) { onPhone(true) }
            Tile("Document", Icons.Rounded.Description, mace.names[2]) { onPhone(false) }
            if (agentFolder != null) Tile("Folder", Icons.Rounded.FolderOpen, mace.names[1]) { onPick(agentFolder) }
            Tile("Hub files", Icons.Rounded.Folder, mace.names[4]) { onPick(".") }
        }
    }
}

/** Browses the hub's files. Picking a file, or a whole folder, attaches it. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FilePicker(start: String, list: suspend (String) -> List<Entry>, onPick: (String) -> Unit, onDismiss: () -> Unit) {
    var path by remember { mutableStateOf(start.trim('/').ifEmpty { "." }) }
    var entries by remember { mutableStateOf<List<Entry>?>(null) }
    var problem by remember { mutableStateOf<String?>(null) }
    LaunchedEffect(path) {
        entries = null
        problem = null
        runCatching { list(path) }
            .onSuccess { e -> entries = e.sortedWith(compareBy<Entry>({ !it.directory }, { it.name.lowercase() })) }
            .onFailure { problem = it.message ?: "Could not open this folder." }
    }
    ModalBottomSheet(onDismissRequest = onDismiss) {
        Column(Modifier.navigationBarsPadding()) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { path = path.substringBeforeLast('/', ".") }, enabled = path != ".") { Icon(Icons.AutoMirrored.Rounded.ArrowBack, "Parent folder") }
                Text("/" + if (path == ".") "" else path, fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
            }
            if (path != ".") {
                FilledTonalButton(onClick = { onPick(path.trimEnd('/') + "/") }, modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                    Icon(Icons.Rounded.Folder, null, Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text("Attach this whole folder")
                }
            }
            HorizontalDivider()
            problem?.let { Text(it, color = MaterialTheme.colorScheme.error, modifier = Modifier.padding(16.dp)) }
            LazyColumn(Modifier.heightIn(max = 520.dp)) {
                if (entries == null && problem == null) item { Text("Opening…", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onSurfaceVariant) }
                if (entries?.isEmpty() == true) item { Text("This folder is empty.", Modifier.padding(16.dp), color = MaterialTheme.colorScheme.onSurfaceVariant) }
                items(entries.orEmpty(), key = { it.path }) { e ->
                    ListItem(
                        leadingContent = { Icon(if (e.directory) Icons.Rounded.Folder else Icons.Rounded.InsertDriveFile, null) },
                        headlineContent = { Text(e.name, maxLines = 1, overflow = TextOverflow.Ellipsis) },
                        supportingContent = { Text(if (e.directory) "Folder" else "${(e.size + 1023) / 1024} KB") },
                        modifier = Modifier.clickable { if (e.directory) path = e.path else onPick(e.path) },
                    )
                }
            }
        }
    }
}

/** Puts a suggestion's text in the draft with the caret at its end. */
fun TextFieldValue.withText(text: String) = TextFieldValue(text, TextRange(text.length))
