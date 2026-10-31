package io.github.phantomconartist.mace

import android.app.Application
import android.os.Build
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import io.github.phantomconartist.mace.data.Hub
import io.github.phantomconartist.mace.data.HubEvent
import io.github.phantomconartist.mace.data.HubException
import io.github.phantomconartist.mace.data.Message
import io.github.phantomconartist.mace.data.Session
import io.github.phantomconartist.mace.data.SignInLink
import io.github.phantomconartist.mace.data.Step
import io.github.phantomconartist.mace.data.events
import io.github.phantomconartist.mace.data.json
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.Job
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull

/** An agent, on this hub or on one of the owner's other machines. */
data class Member(val id: String, val name: String, val handle: String, val provider: String, val status: String, val machine: String?, val folder: String = "", val model: String = "")

/** One row of the chat list. */
data class Conversation(
    val id: String,
    val name: String,
    val preview: String,
    val unread: Int,
    val group: Boolean,
    val at: String,
    val working: Boolean,
)

enum class Filter(val label: String) { All("All"), Unread("Unread"), Groups("Groups"), Agents("Agents") }

sealed interface Screen {
    data class SignIn(val busy: Boolean = false, val problem: String? = null) : Screen
    data class Home(
        val hub: String,
        val conversations: List<Conversation> = emptyList(),
        val loading: Boolean = true,
        val problem: String? = null,
        val filter: Filter = Filter.All,
    ) : Screen
}

/** The open conversation. */
data class ChatState(
    val id: String,
    val name: String,
    val group: Boolean,
    val members: List<Member>,
    val messages: List<Message> = emptyList(),
    val working: Map<String, List<Step>> = emptyMap(),
    val receipts: Map<String, Long> = emptyMap(),
    /** The first message that was unread when the chat opened, for the "New" divider. */
    val firstUnread: Long = 0,
    val loading: Boolean = true,
    val older: Boolean = true,
    val problem: String? = null,
    /** Text to put in the composer, such as an @handle from a swipe. */
    val insert: String? = null,
    /** The conversation's work: running, paused, completed, … */
    val run: String = "",
)

class AppModel(app: Application) : AndroidViewModel(app) {
    private val session = Session(app)
    private val _screen = MutableStateFlow<Screen>(if (session.credential != null) Screen.Home(session.hubName) else Screen.SignIn())
    val screen: StateFlow<Screen> = _screen
    private val _chat = MutableStateFlow<ChatState?>(null)
    val chat: StateFlow<ChatState?> = _chat
    private val _agents = MutableStateFlow<List<Member>>(emptyList())
    /** Every agent the owner can chat with, on every machine. */
    val agents: StateFlow<List<Member>> = _agents

    private var members = mapOf<String, Member>()
    private var groups = mapOf<String, io.github.phantomconartist.mace.data.Room>()
    private var stream: Job? = null
    private var pendingRefresh: Job? = null
    private var working = mapOf<String, Set<String>>()

    init {
        if (session.credential != null) start()
    }

    private fun hub() = Hub(session.origin.orEmpty(), session.credential)

    private fun start() {
        refresh()
        stream?.cancel()
        stream = viewModelScope.launch {
            try {
                hub().events().collect(::onEvent)
            } catch (e: HubException) {
                if (e.status == 401) signOut("This phone was signed out of the hub. Sign in again.")
            }
        }
    }

    // Signing in

    fun signIn(text: String) {
        val link = SignInLink.parse(text)
        if (link == null) {
            _screen.value = Screen.SignIn(problem = "Enter your hub's address, such as your-pc.your-tailnet.ts.net, or paste a sign-in link.")
            return
        }
        _screen.value = Screen.SignIn(busy = true)
        viewModelScope.launch {
            try {
                val health = Hub(link.origin).health()
                if (health.application != "mace" || health.api < BuildConfig.MACE_API) {
                    _screen.value = Screen.SignIn(problem = "This hub runs an older MACE. Update it, then try again.")
                    return@launch
                }
                val grant = Hub(link.origin).signIn(link.code, "MACE on ${Build.MODEL}")
                session.save(link.origin, grant.credential, grant.name)
                _screen.value = Screen.Home(grant.name)
                start()
            } catch (e: IOException) {
                _screen.value = Screen.SignIn(problem = explain(e))
            }
        }
    }

    fun signOut(problem: String? = null) {
        stream?.cancel()
        session.clear()
        _chat.value = null
        _screen.value = Screen.SignIn(problem = problem)
    }

    // The chat list

    fun setFilter(filter: Filter) {
        _screen.update { (it as? Screen.Home)?.copy(filter = filter) ?: it }
    }

    fun refresh() {
        val current = _screen.value as? Screen.Home ?: return
        _screen.value = current.copy(loading = true, problem = null)
        viewModelScope.launch {
            try {
                loadDirectory(remote = false)
                loadInbox()
                // Agents on other machines fill in afterwards; an offline
                // machine must not hold up the list.
                launch { runCatching { loadDirectory(remote = true); loadInbox() } }
            } catch (e: HubException) {
                if (e.status == 401) signOut("This phone was signed out of the hub. Sign in again.") else
                    _screen.update { (it as? Screen.Home)?.copy(loading = false, problem = explain(e)) ?: it }
            } catch (e: IOException) {
                _screen.update { (it as? Screen.Home)?.copy(loading = false, problem = explain(e)) ?: it }
            }
        }
    }

    private suspend fun loadDirectory(remote: Boolean = true) {
        val hub = hub()
        val found = members.filterValues { it.machine != null }.toMutableMap()
        hub.agents().filter { it.status != "removed" }.forEach { found[it.id] = Member(it.id, it.name, it.handle, it.provider, it.status, null, it.folder, it.model) }
        // Agents on other machines carry the machine's ID before "::".
        if (remote) viewModelScope.run {
            hub.peers().map { p ->
                async { runCatching { hub.peerAgents(p.id) }.getOrDefault(emptyList()).filter { it.status != "removed" }.map { a -> Member(a.id, a.name, a.handle, a.provider, a.status, p.name, a.folder, a.model) } }
            }.awaitAll().flatten().forEach { found[it.id] = it }
        }
        members = found
        _agents.value = found.values.sortedBy { it.name.lowercase() }
        groups = hub.rooms().associateBy { it.id }
    }

    private suspend fun loadInbox() {
        val seen = session.seen()
        // Conversations of deleted groups and agents keep their history on
        // the hub but are not listed. Agents on other machines are kept
        // while the machine is out of reach.
        val list = hub().inbox(seen).filter { it.room in groups || it.room in members || "::" in it.room }.map { e ->
            val room = e.room
            val unread = if (_chat.value?.id == room) 0 else e.unread
            Conversation(room, nameOf(room), preview(e.latest), unread, room in groups, e.latest.at, working[room].orEmpty().isNotEmpty())
        }.sortedByDescending { it.at }
        _screen.update { (it as? Screen.Home)?.copy(conversations = list, loading = false, hub = session.hubName) ?: it }
    }

    /** Refreshes the chat list soon, once, however many events ask for it. */
    private fun refreshSoon(directory: Boolean = false) {
        pendingRefresh?.cancel()
        pendingRefresh = viewModelScope.launch {
            delay(400)
            runCatching {
                if (directory) loadDirectory()
                loadInbox()
            }
        }
    }

    // The open conversation

    fun open(id: String) {
        val members = membersOf(id)
        _chat.value = ChatState(id, nameOf(id), id in groups, members)
        viewModelScope.launch {
            try {
                val hub = hub()
                val page = hub.messages(id)
                val receipts = runCatching { hub.receipts(id) }.getOrDefault(emptyMap())
                val run = runCatching { hub.runs().firstOrNull { it.id == id }?.status }.getOrNull().orEmpty()
                val seen = session.seen()[id] ?: 0
                val firstUnread = page.firstOrNull { it.seq > seen && it.sender != "you" }?.seq ?: 0
                _chat.update { c -> c?.takeIf { it.id == id }?.let { it.copy(messages = merge(it.messages, page), receipts = receipts, run = run, firstUnread = firstUnread, loading = false, older = page.size >= 60) } }
                markSeen()
            } catch (e: IOException) {
                _chat.update { c -> c?.takeIf { it.id == id }?.copy(loading = false, problem = explain(e)) }
            }
        }
    }

    fun close() {
        markSeen()
        _chat.value = null
        refreshSoon()
    }

    fun loadOlder() {
        val c = _chat.value ?: return
        val first = c.messages.firstOrNull { it.seq > 0 }?.seq ?: return
        if (!c.older) return
        viewModelScope.launch {
            runCatching { hub().messages(c.id, before = first) }.onSuccess { page ->
                _chat.update { it?.takeIf { s -> s.id == c.id }?.copy(messages = merge(page, it.messages), older = page.size >= 60) }
            }
        }
    }

    /** Puts @handle into the composer, to address one agent in a group. */
    fun mention(member: String) {
        val handle = members[member]?.handle?.takeIf { it.isNotEmpty() } ?: return
        _chat.update { it?.copy(insert = "@$handle ") }
    }

    fun inserted() {
        _chat.update { it?.copy(insert = null) }
    }

    /** Sends the owner's message, showing it at once and confirming when the hub has it. */
    fun send(text: String, file: String = "") {
        val c = _chat.value ?: return
        val body = text.trim()
        if (body.isEmpty() && file.isEmpty()) return
        val clientId = UUID.randomUUID().toString().replace("-", "")
        val pending = Message(id = "pending-$clientId", room = c.id, sender = "you", text = body, file = file, kind = "pending", clientId = clientId, at = java.time.Instant.now().toString())
        _chat.update { it?.copy(messages = it.messages + pending, problem = null) }
        post(c.id, pending)
    }

    /** Tries a failed message again with the same client ID, so it is posted once. */
    fun retry(message: Message) {
        val c = _chat.value ?: return
        val again = message.copy(kind = "pending")
        _chat.update { s -> s?.copy(messages = s.messages.map { if (it.id == message.id) again else it }) }
        post(c.id, again)
    }

    private fun post(room: String, pending: Message) {
        viewModelScope.launch {
            try {
                val sent = hub().send(room, pending.text, pending.clientId, pending.file)
                _chat.update { s -> s?.takeIf { it.id == room }?.copy(messages = merge(s.messages.filter { it.id != pending.id }, listOf(sent))) }
                markSeen()
            } catch (e: IOException) {
                _chat.update { s ->
                    s?.takeIf { it.id == room }?.copy(
                        messages = s.messages.map { if (it.id == pending.id) it.copy(kind = "failed") else it },
                        problem = explain(e),
                    )
                }
            }
        }
    }

    private fun markSeen() {
        val c = _chat.value ?: return
        c.messages.lastOrNull { it.seq > 0 }?.let { session.markSeen(c.id, it.seq) }
    }

    // Commands, files, and controls

    /** Runs a slash command for the agent of the open private chat. */
    fun command(name: String, arg: String) {
        val c = _chat.value ?: return
        viewModelScope.launch {
            try {
                val r = hub().command(c.id, name, arg)
                r.session?.let { a -> members[c.id]?.let { m -> members = members + (c.id to m.copy(name = a.name, handle = a.handle, status = a.status, folder = a.folder, model = a.model)) } }
                _chat.update { s -> s?.takeIf { it.id == c.id }?.copy(messages = merge(s.messages, listOfNotNull(r.note)), members = membersOf(c.id), name = nameOf(c.id), problem = null) }
            } catch (e: IOException) {
                _chat.update { s -> s?.copy(problem = explain(e)) }
            }
        }
    }

    suspend fun files(path: String): List<io.github.phantomconartist.mace.data.Entry> = hub().files(path)

    suspend fun models(agent: String): io.github.phantomconartist.mace.data.Models = hub().models(agent)

    /** Pauses the open conversation's work, or lets it continue. */
    fun setPaused(paused: Boolean) {
        val c = _chat.value ?: return
        viewModelScope.launch {
            try {
                if (paused) hub().pause(c.id) else hub().resume(c.id)
                _chat.update { s -> s?.takeIf { it.id == c.id }?.copy(run = if (paused) "paused" else "running", problem = null) }
            } catch (e: IOException) {
                _chat.update { s -> s?.copy(problem = explain(e)) }
            }
        }
    }

    /** stop, terminate, or resume an agent. */
    fun control(agent: String, action: String) {
        viewModelScope.launch {
            try {
                val a = hub().control(agent, action)
                members[agent]?.let { members = members + (agent to it.copy(status = a.status)) }
                _chat.update { s -> s?.copy(members = s.members.map { if (it.id == agent) it.copy(status = a.status) else it }, problem = null) }
            } catch (e: IOException) {
                _chat.update { s -> s?.copy(problem = explain(e)) }
            }
        }
    }

    // Live events

    private fun onEvent(e: HubEvent) {
        val room = e.data["room"]?.jsonPrimitive?.contentOrNullSafe()
        when (e.type) {
            "message" -> {
                val m = runCatching { json.decodeFromJsonElement(Message.serializer(), e.data) }.getOrNull() ?: return
                _chat.update { c ->
                    if (c == null || c.id != m.room) c else c.copy(
                        messages = merge(c.messages.filter { it.clientId.isEmpty() || it.clientId != m.clientId || it.seq > 0 }, listOf(m)),
                        working = if (m.sender != "you") c.working - m.sender else c.working,
                    )
                }
                if (_chat.value?.id == m.room) markSeen()
                refreshSoon()
            }
            "activity" -> {
                val member = e.data["member"]?.jsonPrimitive?.contentOrNullSafe() ?: return
                room ?: return
                val done = e.data["done"]?.jsonPrimitive?.booleanOrNull == true
                val steps = e.data["steps"]?.let { runCatching { json.decodeFromJsonElement(kotlinx.serialization.builtins.ListSerializer(Step.serializer()), it) }.getOrNull() }.orEmpty()
                working = working + (room to (working[room].orEmpty().let { if (done) it - member else it + member }))
                _chat.update { c -> if (c == null || c.id != room) c else c.copy(working = if (done) c.working - member else c.working + (member to steps)) }
                _screen.update { s -> (s as? Screen.Home)?.copy(conversations = s.conversations.map { if (it.id == room) it.copy(working = working[room].orEmpty().isNotEmpty()) else it }) ?: s }
            }
            "cursor" -> {
                val member = e.data["member"]?.jsonPrimitive?.contentOrNullSafe() ?: return
                val seq = e.data["seq"]?.jsonPrimitive?.longOrNull ?: return
                _chat.update { c -> if (c == null || c.id != room) c else c.copy(receipts = c.receipts + (member to seq)) }
            }
            "room", "session" -> refreshSoon(directory = true)
            "run" -> {
                val id = e.data["id"]?.jsonPrimitive?.contentOrNullSafe()
                val status = e.data["status"]?.jsonPrimitive?.contentOrNullSafe() ?: return
                _chat.update { c -> if (c == null || c.id != id) c else c.copy(run = status) }
            }
            "records" -> {
                val conversation = e.data["conversation"]?.jsonPrimitive?.contentOrNullSafe()
                _chat.value?.takeIf { it.id == conversation || it.id.endsWith("::$conversation") }?.let { open(it.id) }
                refreshSoon()
            }
            "reset" -> {
                refreshSoon(directory = true)
                _chat.value?.let { open(it.id) }
            }
        }
    }

    // Names and previews

    fun memberOf(id: String): Member? = members[id]

    private fun membersOf(id: String): List<Member> =
        (groups[id]?.members ?: listOf(id)).mapNotNull { members[it] }

    private fun nameOf(id: String): String =
        groups[id]?.name ?: members[id]?.let { m -> m.machine?.let { "${m.name} · $it" } ?: m.name } ?: id.substringAfter("::")

    private fun preview(m: Message): String {
        val who = when (m.sender) {
            "you" -> "You: "
            "mace", m.room -> ""
            else -> members[m.sender]?.let { "${it.name}: " }.orEmpty()
        }
        val body = m.text.ifBlank { if (m.file.isNotEmpty()) "Shared ${m.file.trimEnd('/').substringAfterLast('/')}" else "" }
        return (who + body).lineSequence().firstOrNull { it.isNotBlank() }.orEmpty()
    }

    private fun explain(e: IOException): String = when (e) {
        is HubException -> e.message.orEmpty()
        else -> "Could not reach the hub. Check that this phone is on your Tailscale network."
    }
}

/** Merges pages of messages by ID, in conversation order; unsent ones stay last. */
private fun merge(a: List<Message>, b: List<Message>): List<Message> {
    val byId = LinkedHashMap<String, Message>()
    (a + b).forEach { byId[it.id] = it }
    val (sent, unsent) = byId.values.partition { it.seq > 0 }
    return sent.sortedBy { it.seq } + unsent
}

private fun JsonPrimitive.contentOrNullSafe(): String? = if (isString) content else null
