package io.github.phantomconartist.mace.data

import java.io.IOException
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody

// The client API this app speaks is docs/API.md, version 1. Field names
// match the hub's JSON; unknown fields are ignored so newer hubs still work.

@Serializable
data class Health(val application: String = "", val version: String = "", val api: Int = 0, val role: String = "")

@Serializable
data class DeviceGrant(val credential: String, val api: Int = 0, val name: String = "")

@Serializable
data class Agent(
    val id: String,
    val name: String,
    val handle: String = "",
    val provider: String = "",
    val status: String = "",
    @kotlinx.serialization.SerialName("working_dir") val folder: String = "",
    val model: String = "",
    val role: String = "",
    /** When the agent's photo last changed; 0 when it has none. */
    val photo: Long = 0,
)

/** A file or folder on the hub; path is relative to the hub's shared root. */
@Serializable
data class Entry(val name: String, val path: String, val directory: Boolean = false, val size: Long = 0)

@Serializable
data class ModelOption(val id: String, val label: String = "")

@Serializable
data class Models(val models: List<ModelOption> = emptyList(), val current: String = "")

/** A slash command's result: MACE's note in the chat and the agent as it is now. */
@Serializable
data class CommandResult(val note: Message? = null, val session: Agent? = null)

@Serializable
private data class CommandRequest(val name: String, val arg: String)

@Serializable
private data class ControlRequest(val action: String)

@Serializable
data class Room(
    val id: String,
    val name: String,
    val members: List<String> = emptyList(),
    /** Roles the owner set in this group. */
    val roles: Map<String, String> = emptyMap(),
    /** Roles agents chose for themselves. */
    val claimed: Map<String, String> = emptyMap(),
    val parent: String = "",
    val photo: Long = 0,
)

@Serializable
data class Peer(val id: String, val name: String = "")

/** How a conversation's agents work: reply, goal, continuous, or team. */
@Serializable
data class Policy(val mode: String = "reply", val goal: String = "", val limit: Int = 8, val tone: String = "", val lead: String = "")

/** An agent CLI installed on a machine. */
@Serializable
data class Provider(val id: String, val name: String = "", val available: Boolean = false)

@Serializable
data class Catalog(val providers: List<Provider> = emptyList())

/** A machine joined to this hub. */
@Serializable
data class Node(
    val id: String,
    val name: String = "",
    val self: Boolean = false,
    val available: Boolean = false,
    @kotlinx.serialization.SerialName("last_seen") val lastSeen: String = "",
    val problem: String = "",
    val outdated: Boolean = false,
)

/** A phone or browser signed in to the hub. */
@Serializable
data class Device(val id: String, val name: String = "", val created: String = "", @kotlinx.serialization.SerialName("last_seen") val lastSeen: String = "", val current: Boolean = false)

@Serializable
data class PresenceSettings(@kotlinx.serialization.SerialName("keep_working") val keepWorking: Boolean = false)

@Serializable
data class Uploaded(val file: String, val size: Long = 0)

@Serializable
data class SignInQr(val link: String = "", @kotlinx.serialization.SerialName("expires_in") val expiresIn: Int = 600)

/** One step an agent took while replying: reading a file, running a command. */
@Serializable
data class Step(val id: String = "", val kind: String = "", val title: String = "", val detail: String = "", val state: String = "")

@Serializable
data class Message(
    val id: String,
    val room: String = "",
    val sender: String = "",
    val text: String = "",
    val file: String = "",
    val kind: String = "",
    val flair: String = "",
    val at: String = "",
    val seq: Long = 0,
    @kotlinx.serialization.SerialName("client_id") val clientId: String = "",
    val mentions: List<String> = emptyList(),
    val activity: List<Step> = emptyList(),
    /** Delivery state per target, such as {"conversation": "delivered"}. */
    val delivery: Map<String, String> = emptyMap(),
)

@Serializable
private data class SendRequest(val text: String, val file: String, @kotlinx.serialization.SerialName("client_id") val clientId: String)

@Serializable
data class RunState(val id: String, val status: String = "", val replies: Int = 0, val error: String = "")

@Serializable
data class InboxEntry(val room: String, val latest: Message, val unread: Int = 0)

@Serializable
private data class NewGroup(val name: String, val members: List<String>)

@Serializable
private data class MemberRequest(val member: String)

@Serializable
private data class RoleRequest(val role: String)

@Serializable
private data class ProfileRequest(val name: String, val handle: String, val role: String)

@Serializable
private data class NewAgent(val provider: String, @kotlinx.serialization.SerialName("working_dir") val folder: String)

@Serializable
private data class SignInRequest(val code: String, val name: String)

@Serializable
private data class InboxRequest(val seen: Map<String, Long>)

/** A refusal or failure, with the hub's own words when it gave any. */
class HubException(message: String, val status: Int = 0) : IOException(message)

/** The link the hub shows as a sign-in QR: https://host/#connect-once=<code>. */
data class SignInLink(val origin: String, val code: String) {
    companion object {
        private val pattern = Regex("""^(https?://[^/#?\s]+)[^#]*#connect-once=([0-9a-fA-F]{16,128})$""")

        private val address = Regex("""^(https?://)?([A-Za-z0-9.-]+\.[A-Za-z]{2,}|\d{1,3}(\.\d{1,3}){3})(:\d+)?/?$""")

        /**
         * A sign-in link, or just the hub's address. An address has no code:
         * the hub then signs in only the owner's own devices on Tailscale.
         */
        fun parse(text: String): SignInLink? {
            val t = text.trim()
            pattern.find(t)?.let { return SignInLink(it.groupValues[1].trimEnd('/'), it.groupValues[2]) }
            if (!address.matches(t)) return null
            val origin = if (t.startsWith("http")) t.trimEnd('/') else "https://" + t.trimEnd('/')
            return SignInLink(origin, "")
        }
    }
}

internal val json = Json { ignoreUnknownKeys = true; explicitNulls = false }
private val jsonType = "application/json".toMediaType()

private val sharedClient = OkHttpClient.Builder()
    .connectTimeout(10, TimeUnit.SECONDS)
    .readTimeout(30, TimeUnit.SECONDS)
    .build()

// The stream stays open; the hub sends events as they happen. A ping
// interval notices a dead connection without a read timeout.
private val streamClient = sharedClient.newBuilder()
    .readTimeout(0, TimeUnit.SECONDS)
    .pingInterval(20, TimeUnit.SECONDS)
    .build()

/** One hub, reached at origin, optionally signed in with a device credential. */
class Hub(val origin: String, private val credential: String? = null, private val http: OkHttpClient = sharedClient) {

    suspend fun health(): Health = get("/health")

    /** Trades a one-time sign-in code for this device's credential. */
    suspend fun signIn(code: String, deviceName: String): DeviceGrant =
        post("/api/device", json.encodeToString(SignInRequest.serializer(), SignInRequest(code, deviceName)))

    suspend fun agents(): List<Agent> = get("/api/v1/sessions")

    suspend fun rooms(): List<Room> = get("/api/v1/rooms")

    suspend fun peers(): List<Peer> = get("/api/v1/peers")

    suspend fun peerAgents(peer: String): List<Agent> = get("/api/v1/peers/$peer/sessions")

    /** A page of a conversation, oldest to newest; before pages back through history. */
    suspend fun messages(room: String, before: Long = 0, limit: Int = 60): List<Message> =
        get("/api/v1/chats/${enc(room)}/messages?limit=$limit" + if (before > 0) "&before=$before" else "")

    /** Posts the owner's message. The same clientId is posted once, so retries are safe. */
    suspend fun send(room: String, text: String, clientId: String, file: String = ""): Message =
        post("/api/v1/chats/${enc(room)}/messages", json.encodeToString(SendRequest.serializer(), SendRequest(text, file, clientId)))

    /** A folder listing on the hub; path "." is the shared root. */
    suspend fun files(path: String): List<Entry> = get("/api/v1/files?path=${enc(path)}")

    suspend fun models(agent: String): Models = get("/api/v1/sessions/${enc(agent)}/models")

    suspend fun command(agent: String, name: String, arg: String): CommandResult =
        post("/api/v1/sessions/${enc(agent)}/command", json.encodeToString(CommandRequest.serializer(), CommandRequest(name, arg)))

    /** stop, terminate, remove, or resume an agent. */
    suspend fun control(agent: String, action: String): Agent =
        post("/api/v1/sessions/${enc(agent)}/control", json.encodeToString(ControlRequest.serializer(), ControlRequest(action)))

    suspend fun pause(room: String): kotlinx.serialization.json.JsonElement = post("/api/v1/chats/${enc(room)}/pause", "{}")

    suspend fun resume(room: String): kotlinx.serialization.json.JsonElement = post("/api/v1/chats/${enc(room)}/continue", "{}")

    suspend fun runs(): List<RunState> = get("/api/v1/runs")

    /** How far each member has read, by message seq. */
    suspend fun receipts(room: String): Map<String, Long> = get("/api/v1/chats/${enc(room)}/receipts")

    /** Opens the live event stream; the caller reads and closes it. */
    fun openStream(lastId: String?): okhttp3.Response {
        val builder = Request.Builder().url("$origin/api/v1/stream").header("Accept", "text/event-stream")
        credential?.let { builder.header("Authorization", "Device $it") }
        lastId?.let { builder.header("Last-Event-ID", it) }
        val res = streamClient.newCall(builder.build()).execute()
        if (!res.isSuccessful) {
            val text = res.body.string()
            res.close()
            throw HubException(text.trim().ifEmpty { "The hub answered ${res.code}." }, res.code)
        }
        return res
    }

    suspend fun createGroup(name: String, members: List<String>): Room =
        post("/api/v1/rooms", json.encodeToString(NewGroup.serializer(), NewGroup(name, members)))

    /** Removes a group; its history stays on the hub. */
    /** Deletes a conversation's messages on the hub. */
    suspend fun clearChat(room: String): JsonElement = post("/api/v1/chats/${enc(room)}/clear", "{}")

    suspend fun removeGroup(room: String): JsonElement = post("/api/v1/rooms/${enc(room)}/remove", "{}")

    suspend fun addMember(room: String, member: String): JsonElement =
        post("/api/v1/rooms/${enc(room)}/members", json.encodeToString(MemberRequest.serializer(), MemberRequest(member)))

    suspend fun removeMember(room: String, member: String): JsonElement =
        post("/api/v1/rooms/${enc(room)}/members/${enc(member)}/remove", "{}")

    suspend fun setRole(room: String, member: String, role: String): JsonElement =
        post("/api/v1/rooms/${enc(room)}/members/${enc(member)}/role", json.encodeToString(RoleRequest.serializer(), RoleRequest(role)))

    suspend fun policy(room: String): Policy = get("/api/v1/chats/${enc(room)}/policy")

    suspend fun setPolicy(room: String, policy: Policy): JsonElement =
        post("/api/v1/chats/${enc(room)}/policy", json.encodeToString(Policy.serializer(), policy))

    suspend fun profile(agent: String, name: String, handle: String, role: String): JsonElement =
        post("/api/v1/sessions/${enc(agent)}/profile", json.encodeToString(ProfileRequest.serializer(), ProfileRequest(name, handle, role)))

    /** Agent CLIs installed on this hub, or on another machine. */
    suspend fun catalog(machine: String? = null): Catalog = get(if (machine == null) "/api/v1/catalog" else "/api/v1/peers/${enc(machine)}/catalog")

    /** Starts an agent in a working folder, on this hub or another machine. */
    suspend fun startAgent(machine: String?, provider: String, folder: String): Agent =
        post(if (machine == null) "/api/v1/sessions" else "/api/v1/peers/${enc(machine)}/sessions", json.encodeToString(NewAgent.serializer(), NewAgent(provider, folder)))

    /** A folder listing on another machine, answered through the hub. */
    suspend fun machineFiles(machine: String, path: String): List<Entry> = get("/api/v1/machines/${enc(machine)}/files?path=${enc(path)}")

    suspend fun nodes(): List<Node> = get("/api/v1/nodes")

    suspend fun devices(): List<Device> = get("/api/v1/devices")

    suspend fun revoke(device: String): JsonElement = post("/api/v1/devices/${enc(device)}/revoke", "{}")

    suspend fun presenceSettings(): PresenceSettings = get("/api/v1/settings/presence")

    suspend fun setKeepWorking(on: Boolean): JsonElement =
        post("/api/v1/settings/presence", json.encodeToString(PresenceSettings.serializer(), PresenceSettings(on)))

    /** A one-time sign-in link for another device. */
    suspend fun signInLink(): SignInQr = get("/api/v1/signin/qr")

    suspend fun signOutHere(): JsonElement = post("/api/session/sign-out", "{}")

    /** Uploads a file from the phone into a conversation; post a message with the answer's file to send it. */
    suspend fun upload(room: String, name: String, type: String, body: okhttp3.RequestBody): Uploaded =
        call(Request.Builder().url("$origin/api/v1/chats/${enc(room)}/upload?name=${enc(name)}").post(body).header("Content-Type", type.ifEmpty { "application/octet-stream" }))

    /** Sets an agent's or a group's photo. */
    suspend fun setPhoto(id: String, group: Boolean, image: ByteArray): JsonElement =
        call(Request.Builder().url(origin + photoPath(id, group)).post(image.toRequestBody("image/jpeg".toMediaType())))

    suspend fun removePhoto(id: String, group: Boolean): JsonElement = call(Request.Builder().url(origin + photoPath(id, group)).delete())

    fun photoPath(id: String, group: Boolean) = if (group) "/api/v1/rooms/${enc(id)}/photo" else "/api/v1/sessions/${enc(id)}/photo"

    /** A file's address on the hub and the header that signs the request. */
    fun fileUrl(path: String) = "$origin/api/v1/files?path=${enc(path)}"

    fun headers(): Map<String, String> = credential?.let { mapOf("Authorization" to "Device $it") }.orEmpty()

    /** Fetches raw bytes, such as a picture, or null when there are none. */
    suspend fun bytes(url: String, limit: Long = 64L shl 20): ByteArray? = withContext(Dispatchers.IO) {
        val b = Request.Builder().url(if (url.startsWith("http")) url else origin + url)
        credential?.let { b.header("Authorization", "Device $it") }
        http.newCall(b.build()).execute().use { res ->
            if (!res.isSuccessful) return@use null
            val body = res.body
            if (body.contentLength() > limit) return@use null
            body.bytes()
        }
    }

    /** A hub file's size in bytes, read from a one-byte range request, or null. */
    suspend fun fileSize(path: String): Long? = withContext(Dispatchers.IO) {
        val b = Request.Builder().url(fileUrl(path)).header("Range", "bytes=0-0")
        credential?.let { b.header("Authorization", "Device $it") }
        runCatching {
            http.newCall(b.build()).execute().use { res ->
                res.header("Content-Range")?.substringAfter('/')?.toLongOrNull() ?: res.header("Content-Length")?.toLongOrNull()?.takeIf { res.code == 200 }
            }
        }.getOrNull()
    }

    /** Saves a hub file to target, for opening in another app. */
    suspend fun download(path: String, target: java.io.File) = withContext(Dispatchers.IO) {
        val b = Request.Builder().url(fileUrl(path))
        credential?.let { b.header("Authorization", "Device $it") }
        http.newCall(b.build()).execute().use { res ->
            if (!res.isSuccessful) throw HubException(res.body.string().trim().ifEmpty { "Could not download this file." }, res.code)
            target.parentFile?.mkdirs()
            target.outputStream().use { res.body.byteStream().copyTo(it) }
        }
    }

    private fun enc(segment: String) = java.net.URLEncoder.encode(segment, "UTF-8").replace("+", "%20")

    /** The conversation list: each conversation's newest message and unread count since seen. */
    suspend fun inbox(seen: Map<String, Long> = emptyMap()): List<InboxEntry> =
        post("/api/v1/inbox", json.encodeToString(InboxRequest.serializer(), InboxRequest(seen)))

    private suspend inline fun <reified T> get(path: String): T = call(Request.Builder().url(origin + path).get())

    private suspend inline fun <reified T> post(path: String, body: String): T =
        call(Request.Builder().url(origin + path).post(body.toRequestBody(jsonType)))

    private suspend inline fun <reified T> call(builder: Request.Builder): T = withContext(Dispatchers.IO) {
        credential?.let { builder.header("Authorization", "Device $it") }
        http.newCall(builder.build()).execute().use { res ->
            val text = res.body.string()
            if (!res.isSuccessful) {
                throw HubException(text.trim().ifEmpty { "The hub answered ${res.code}." }, res.code)
            }
            // Some actions answer with no body.
            json.decodeFromString<T>(text.ifBlank { "null" })
        }
    }
}
