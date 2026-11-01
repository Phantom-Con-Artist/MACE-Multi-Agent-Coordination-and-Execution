package io.github.phantomconartist.mace.ui

import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import io.github.phantomconartist.mace.Member
import org.junit.Assert.assertEquals
import org.junit.Test

class ComposerTest {
    private fun member(id: String, handle: String, machine: String? = null) =
        Member(id, "Claude", handle, "claude", "ready", machine)

    @Test fun sameHandleOnTwoMachinesListsBothAgents() {
        val members = listOf(member("a1", "claude"), member("pc::a1", "claude", "pc"), member("a1", "claude"))
        val got = mentionSuggestions(TextFieldValue("hi @cl", TextRange(6)), members)
        assertEquals(listOf("a1", "pc::a1"), got.map { it.id })
        // A shared handle is told apart the way the hub does: claude@here, claude@pc.
        assertEquals(listOf("hi @claude@here ", "hi @claude@pc "), got.map { it.text })
    }

    @Test fun uniqueHandleStaysPlain() {
        val handles = roomHandles(listOf(member("a1", "agy"), member("pc::m1", "muse", "My PC")))
        assertEquals(mapOf("a1" to "agy", "pc::m1" to "muse"), handles)
    }

    @Test fun remoteHandleUsesTheMachineSlug() {
        val handles = roomHandles(listOf(member("a1", "agy"), member("p::a5", "agy", "My PC")))
        assertEquals("agy@my-pc", handles["p::a5"])
    }

    @Test fun bareAtOffersEveryoneFirst() {
        val got = mentionSuggestions(TextFieldValue("@", TextRange(1)), listOf(member("a1", "claude")))
        assertEquals(listOf("@all", "a1"), got.map { it.id })
    }
}
