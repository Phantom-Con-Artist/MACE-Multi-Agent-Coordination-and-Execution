package io.github.phantomconartist.mace.data

import android.content.Context

/**
 * The hub this phone is signed in to. The credential lives in app-private
 * storage and is excluded from backups (res/xml/data_extraction_rules.xml).
 */
class Session(context: Context) {
    private val prefs = context.getSharedPreferences("hub", Context.MODE_PRIVATE)
    private val seenPrefs = context.getSharedPreferences("seen", Context.MODE_PRIVATE)

    val origin: String? get() = prefs.getString("origin", null)
    val credential: String? get() = prefs.getString("credential", null)
    val hubName: String get() = prefs.getString("name", null).orEmpty()

    fun save(origin: String, credential: String, name: String) {
        prefs.edit().putString("origin", origin).putString("credential", credential).putString("name", name).apply()
    }

    /** Observe mode shows each agent's tool calls and file edits; messenger mode only messages. */
    var observe: Boolean
        get() = prefs.getBoolean("observe", false)
        set(v) = prefs.edit().putBoolean("observe", v).apply()

    var sounds: Boolean
        get() = prefs.getBoolean("sounds", true)
        set(v) = prefs.edit().putBoolean("sounds", v).apply()

    /** The newest message seq the owner has seen, per conversation, for unread counts. */
    fun seen(): Map<String, Long> =
        seenPrefs.all.mapNotNull { (k, v) -> (v as? Long)?.let { k to it } }.toMap()

    fun markSeen(room: String, seq: Long) {
        if (seq > (seenPrefs.getLong(room, 0))) seenPrefs.edit().putLong(room, seq).apply()
    }

    /** Unlinks this phone. The hub's name and what was read stay, for the saved chats. */
    fun clear() {
        prefs.edit().remove("credential").apply()
    }
}
