package io.github.phantomconartist.mace.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.isActive
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject

/** One Server-Sent Event from the hub; see docs/API.md, Live updates. */
data class HubEvent(val type: String, val data: JsonObject)

/**
 * The hub's live events, kept open for as long as the flow is collected.
 * A dropped connection reconnects with Last-Event-ID, so nothing is missed;
 * when the hub cannot replay (a "reset" event), the app refetches.
 */
fun Hub.events(): Flow<HubEvent> = flow {
    var lastId: String? = null
    var attempt = 0
    while (currentCoroutineContext().isActive) {
        try {
            openStream(lastId).use { res ->
                attempt = 0
                val source = res.body.source()
                var type = "message"
                val data = StringBuilder()
                while (true) {
                    val line = source.readUtf8Line() ?: break
                    when {
                        line.isEmpty() -> {
                            if (data.isNotEmpty()) {
                                runCatching { json.parseToJsonElement(data.toString()).jsonObject }
                                    .getOrNull()?.let { emit(HubEvent(type, it)) }
                            }
                            type = "message"
                            data.clear()
                        }
                        line.startsWith("id:") -> lastId = line.substring(3).trim()
                        line.startsWith("event:") -> type = line.substring(6).trim()
                        line.startsWith("data:") -> data.append(line.substring(5).trimStart())
                    }
                }
            }
        } catch (e: HubException) {
            if (e.status == 401) throw e
        } catch (_: java.io.IOException) {
        }
        // Back off with jitter, up to 15 seconds, before reconnecting.
        attempt++
        delay((500L shl minOf(attempt, 5)).coerceAtMost(15_000) / 2 + (0..500).random())
    }
}.flowOn(Dispatchers.IO)

