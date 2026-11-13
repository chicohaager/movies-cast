package io.github.chicohaager.moviescast

import io.github.chicohaager.moviescast.core.MediaItem
import io.github.chicohaager.moviescast.core.SubtitleFile

/** One film or episode the player is asked to play, with the subtitle files lying next to it. */
data class QueueEntry(val id: String, val title: String, val subtitles: List<SubtitleFile>) : java.io.Serializable

/**
 * What the player gets for "Play" on [item]: the item and, for an episode, the episodes after it
 * ([following]), so the next one starts by itself. At most [limit] entries: an intent is no place
 * for a series of a thousand episodes.
 */
fun queueFor(
    item: MediaItem,
    following: List<MediaItem>,
    title: (MediaItem) -> String,
    subtitles: (MediaItem) -> List<SubtitleFile>,
    limit: Int = 100,
): List<QueueEntry> =
    (listOf(item) + following.filter { it.playable && it.id != item.id }).take(limit).map { QueueEntry(it.id, title(it), subtitles(it)) }
