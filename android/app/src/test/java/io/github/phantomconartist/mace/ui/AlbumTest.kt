package io.github.phantomconartist.mace.ui

import io.github.phantomconartist.mace.data.Message
import org.junit.Assert.assertEquals
import org.junit.Test

class AlbumTest {
    private fun pic(id: String, sender: String = "a", sec: Int = 0, text: String = "") =
        Message(id, sender = sender, file = "shots/$id.png", text = text, at = "2026-11-01T10:00:%02dZ".format(sec))

    @Test fun fourPicturesInARowBecomeOneAlbum() {
        val rows = albums(listOf(pic("1"), pic("2", sec = 1), pic("3", sec = 2), pic("4", sec = 3), pic("5", sec = 4)))
        assertEquals(listOf(listOf("1", "2", "3", "4", "5")), rows.map { r -> r.map { it.id } })
    }

    @Test fun fewerThanFourStaySeparate() {
        val rows = albums(listOf(pic("1"), pic("2", sec = 1), pic("3", sec = 2)))
        assertEquals(3, rows.size)
    }

    @Test fun anotherSenderOrACaptionBreaksTheAlbum() {
        val rows = albums(listOf(pic("1"), pic("2", sec = 1), pic("x", sender = "b", sec = 2), pic("3", sec = 3), pic("4", sec = 4, text = "look")))
        assertEquals(5, rows.size)
    }
}
