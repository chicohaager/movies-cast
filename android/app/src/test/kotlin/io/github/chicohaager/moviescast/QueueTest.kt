package io.github.chicohaager.moviescast

import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.SubtitleFile
import org.junit.Assert.assertEquals
import org.junit.Test

class QueueTest {
    private fun episode(id: String) = MediaItem(id = id, name = "Name $id", type = "Episode")
    private val sub = SubtitleFile("http://192.0.2.10/sub.srt", "srt", "deu", "German")

    @Test
    fun theItemComesFirstThenTheEpisodesAfterItEachWithItsSubtitles() {
        val queue = queueFor(episode("e2"), listOf(episode("e3"), episode("e4")), { "T " + it.id }, { if (it.id == "e3") listOf(sub) else emptyList() })
        assertEquals(
            listOf(QueueEntry("e2", "T e2", emptyList()), QueueEntry("e3", "T e3", listOf(sub)), QueueEntry("e4", "T e4", emptyList())),
            queue,
        )
    }

    @Test
    fun aFilmAloneIsAQueueOfOne() {
        assertEquals(listOf("f1"), queueFor(MediaItem(id = "f1", name = "Film", type = "Movie"), emptyList(), { it.name }, { emptyList() }).map { it.id })
    }

    @Test
    fun theQueueIsCutAtTheLimitAndNamesNoItemTwiceNorASeries() {
        val following = listOf(episode("e1"), MediaItem(id = "s", name = "Series", type = "Series")) + (2..9).map { episode("e$it") }
        val queue = queueFor(episode("e1"), following, { it.id }, { emptyList() }, limit = 4)
        assertEquals(listOf("e1", "e2", "e3", "e4"), queue.map { it.id })
    }
}
