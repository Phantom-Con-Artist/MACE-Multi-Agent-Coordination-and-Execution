package io.github.phantomconartist.mace.data

import java.io.File
import kotlinx.serialization.Serializable

// The phone keeps the chat list and each chat's recent messages, so the app
// opens with saved conversations even when the hub or a worker is down.
// It lives in app-private storage and is cleared on sign-out.

@Serializable
data class RemoteAgent(val machine: String, val agent: Agent)

@Serializable
data class Snapshot(
    val agents: List<Agent> = emptyList(),
    val remote: List<RemoteAgent> = emptyList(),
    val rooms: List<Room> = emptyList(),
    val inbox: List<InboxEntry> = emptyList(),
)

class Cache(root: File) {
    private val dir = File(root, "saved").also { it.mkdirs() }

    fun snapshot(): Snapshot? = read(File(dir, "snapshot.json")) { json.decodeFromString(Snapshot.serializer(), it) }

    fun saveSnapshot(s: Snapshot) = write(File(dir, "snapshot.json"), json.encodeToString(Snapshot.serializer(), s))

    fun messages(room: String): List<Message>? =
        read(roomFile(room)) { json.decodeFromString(kotlinx.serialization.builtins.ListSerializer(Message.serializer()), it) }

    fun saveMessages(room: String, list: List<Message>) =
        write(roomFile(room), json.encodeToString(kotlinx.serialization.builtins.ListSerializer(Message.serializer()), list.filter { it.seq > 0 }.takeLast(120)))

    fun clear() { dir.deleteRecursively(); dir.mkdirs() }

    private fun roomFile(room: String) = File(dir, "chat-" + room.hashCode().toUInt().toString(16) + ".json")

    private fun <T> read(f: File, decode: (String) -> T): T? = runCatching { if (f.exists()) decode(f.readText()) else null }.getOrNull()

    private fun write(f: File, text: String) {
        runCatching { File(f.path + ".tmp").apply { writeText(text) }.renameTo(f) }
    }
}
