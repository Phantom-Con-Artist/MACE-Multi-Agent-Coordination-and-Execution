package io.github.phantomconartist.mace

import android.app.Application
import android.os.Build
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import io.github.phantomconartist.mace.data.Hub
import io.github.phantomconartist.mace.data.HubException
import io.github.phantomconartist.mace.data.InboxEntry
import io.github.phantomconartist.mace.data.Session
import io.github.phantomconartist.mace.data.SignInLink
import java.io.IOException
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.launch

/** One row of the chat list. */
data class Conversation(val id: String, val name: String, val preview: String, val unread: Int, val group: Boolean)

sealed interface Screen {
    data class SignIn(val busy: Boolean = false, val problem: String? = null) : Screen
    data class Chats(
        val hub: String,
        val conversations: List<Conversation> = emptyList(),
        val loading: Boolean = true,
        val problem: String? = null,
    ) : Screen
}

class AppModel(app: Application) : AndroidViewModel(app) {
    private val session = Session(app)
    private val _screen = MutableStateFlow<Screen>(
        if (session.credential != null) Screen.Chats(session.hubName) else Screen.SignIn(),
    )
    val screen: StateFlow<Screen> = _screen

    init {
        if (session.credential != null) refresh()
    }

    private fun hub() = Hub(session.origin.orEmpty(), session.credential)

    /** Signs in with the link the hub shows under Settings, Phone. */
    fun signIn(text: String) {
        val link = SignInLink.parse(text)
        if (link == null) {
            _screen.value = Screen.SignIn(problem = "That is not a MACE sign-in link. It looks like https://your-hub/#connect-once=…")
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
                _screen.value = Screen.Chats(grant.name)
                refresh()
            } catch (e: IOException) {
                _screen.value = Screen.SignIn(problem = explain(e))
            }
        }
    }

    fun refresh() {
        val current = _screen.value as? Screen.Chats ?: return
        _screen.value = current.copy(loading = true, problem = null)
        viewModelScope.launch {
            try {
                val hub = hub()
                val names = mutableMapOf<String, String>()
                val groups = mutableSetOf<String>()
                hub.rooms().forEach { names[it.id] = it.name; groups += it.id }
                hub.agents().forEach { names[it.id] = it.name }
                // Agents on other machines carry the machine's ID before "::".
                hub.peers().map { p -> async { runCatching { hub.peerAgents(p.id) }.getOrDefault(emptyList()) } }
                    .awaitAll().flatten().forEach { names[it.id] = it.name }
                val list = hub.inbox().sortedByDescending { it.latest.at }.map { it.toConversation(names, groups) }
                _screen.value = Screen.Chats(session.hubName, list, loading = false)
            } catch (e: HubException) {
                if (e.status == 401) signOut("This phone was signed out of the hub. Sign in again.") else
                    _screen.value = current.copy(loading = false, problem = explain(e))
            } catch (e: IOException) {
                _screen.value = current.copy(loading = false, problem = explain(e))
            }
        }
    }

    fun signOut(problem: String? = null) {
        session.clear()
        _screen.value = Screen.SignIn(problem = problem)
    }

    private fun InboxEntry.toConversation(names: Map<String, String>, groups: Set<String>): Conversation {
        val who = when (latest.sender) {
            "you" -> "You: "
            "mace", room -> ""
            else -> names[latest.sender]?.let { "$it: " }.orEmpty()
        }
        val body = latest.text.ifBlank { if (latest.file.isNotEmpty()) "Shared ${latest.file.trimEnd('/').substringAfterLast('/')}" else "" }
        return Conversation(room, names[room] ?: room.substringAfter("::"), (who + body).lineSequence().first(), unread, room in groups)
    }

    private fun explain(e: IOException): String = when (e) {
        is HubException -> e.message.orEmpty()
        else -> "Could not reach the hub. Check that this phone is on your Tailscale network."
    }
}
