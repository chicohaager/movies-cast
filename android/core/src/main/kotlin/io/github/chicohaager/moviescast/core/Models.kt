package io.github.chicohaager.moviescast.core

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** What `/System/Info/Public` tells without a login. */
data class ServerInfo(val product: String, val name: String, val version: String)

/** One login at the server. The token never leaves this object except in a request header. */
data class Session(
    val baseUrl: String,
    val token: String,
    val userId: String,
    val userName: String,
) {
    override fun toString(): String = "Session(baseUrl=$baseUrl, userId=$userId, userName=$userName)"
}

/**
 * An audio or subtitle track, as the server lists it. [external] is a subtitle file lying next to
 * the film; [format] is then that file's extension (`srt`), which the server wants in the address.
 */
data class Track(
    val index: Int,
    val codec: String,
    val language: String,
    val title: String,
    val channels: Int,
    val external: Boolean = false,
    val format: String = "",
)

/**
 * A subtitle file the player loads next to the film. [format] is `srt`, `vtt`, `ass` or `ssa`.
 * Serializable, because the player gets its queue through an intent.
 */
data class SubtitleFile(val url: String, val format: String, val language: String, val label: String) : java.io.Serializable

/** The items of this list that follow the one with [id], in order: the episodes still to come. */
fun List<MediaItem>.after(id: String): List<MediaItem> {
    val at = indexOfFirst { it.id == id }
    return if (at < 0) emptyList() else drop(at + 1)
}

/** A film, series or episode as the screens show it. */
data class MediaItem(
    val id: String,
    val name: String,
    val type: String,
    val year: Int = 0,
    val overview: String = "",
    val runtimeSeconds: Long = 0,
    val resumeSeconds: Long = 0,
    val played: Boolean = false,
    val hasPoster: Boolean = false,
    val container: String = "",
    val videoCodec: String = "",
    val width: Int = 0,
    val height: Int = 0,
    val sizeBytes: Long = 0,
    val audio: List<Track> = emptyList(),
    val subtitles: List<Track> = emptyList(),
    val genres: List<String> = emptyList(),
    val rating: String = "",
    val score: Double = 0.0,
    val seriesId: String = "",
    val seriesName: String = "",
    val season: Int = 0,
    val episode: Int = 0,
    val childCount: Int = 0,
    val sourceId: String = "",
    /** When the server took the item in, as it writes it (ISO 8601, so the text sorts like the time). */
    val added: String = "",
    val favorite: Boolean = false,
) {
    /** A series is a folder of episodes; everything else is a file. */
    val playable: Boolean get() = type != "Series"

    /** Position worth offering: not the first seconds and not the credits. */
    val resumable: Boolean
        get() = resumeSeconds > 10 && (runtimeSeconds == 0L || resumeSeconds < runtimeSeconds - 30)
}

/** What the player reports so the server's "continue watching" stays right. */
data class PlayState(
    val itemId: String,
    val playSessionId: String,
    val positionSeconds: Long,
    val paused: Boolean,
)

const val TICKS_PER_SECOND = 10_000_000L

@Serializable
internal data class PublicInfoDto(
    @SerialName("ProductName") val productName: String = "",
    @SerialName("ServerName") val serverName: String = "",
    @SerialName("Version") val version: String = "",
)

@Serializable
internal data class AuthDto(
    @SerialName("AccessToken") val accessToken: String = "",
    @SerialName("User") val user: UserDto = UserDto(),
)

@Serializable
internal data class UserDto(
    @SerialName("Id") val id: String = "",
    @SerialName("Name") val name: String = "",
)

@Serializable
internal data class ItemsDto(
    @SerialName("Items") val items: List<ItemDto> = emptyList(),
    @SerialName("TotalRecordCount") val total: Int = 0,
)

@Serializable
internal data class UserDataDto(
    @SerialName("PlaybackPositionTicks") val positionTicks: Long = 0,
    @SerialName("Played") val played: Boolean = false,
    @SerialName("IsFavorite") val favorite: Boolean = false,
)

@Serializable
internal data class StreamDto(
    @SerialName("Type") val type: String = "",
    @SerialName("Codec") val codec: String = "",
    @SerialName("Language") val language: String = "",
    @SerialName("DisplayTitle") val displayTitle: String = "",
    @SerialName("Index") val index: Int = 0,
    @SerialName("Width") val width: Int = 0,
    @SerialName("Height") val height: Int = 0,
    @SerialName("Channels") val channels: Int = 0,
    @SerialName("IsExternal") val external: Boolean = false,
    @SerialName("Path") val path: String = "",
)

@Serializable
internal data class PlaybackInfoDto(
    @SerialName("MediaSources") val sources: List<TranscodeSourceDto> = emptyList(),
)

@Serializable
internal data class TranscodeSourceDto(
    @SerialName("TranscodingUrl") val transcodingUrl: String = "",
)

@Serializable
internal data class SourceDto(
    @SerialName("Id") val id: String = "",
    @SerialName("Container") val container: String = "",
    @SerialName("Size") val size: Long = 0,
    @SerialName("MediaStreams") val streams: List<StreamDto> = emptyList(),
)

@Serializable
internal data class ItemDto(
    @SerialName("Id") val id: String = "",
    @SerialName("Name") val name: String = "",
    @SerialName("Type") val type: String = "",
    @SerialName("ProductionYear") val year: Int = 0,
    @SerialName("Overview") val overview: String = "",
    @SerialName("RunTimeTicks") val runTimeTicks: Long = 0,
    @SerialName("Container") val container: String = "",
    @SerialName("ImageTags") val imageTags: Map<String, String> = emptyMap(),
    @SerialName("UserData") val userData: UserDataDto = UserDataDto(),
    @SerialName("MediaSources") val sources: List<SourceDto> = emptyList(),
    @SerialName("MediaStreams") val streams: List<StreamDto> = emptyList(),
    @SerialName("Genres") val genres: List<String> = emptyList(),
    @SerialName("OfficialRating") val rating: String = "",
    @SerialName("CommunityRating") val score: Double = 0.0,
    @SerialName("SeriesId") val seriesId: String = "",
    @SerialName("SeriesName") val seriesName: String = "",
    @SerialName("ParentIndexNumber") val parentIndex: Int = 0,
    @SerialName("IndexNumber") val index: Int = 0,
    @SerialName("RecursiveItemCount") val recursiveCount: Int = 0,
    @SerialName("ChildCount") val childCount: Int = 0,
    @SerialName("DateCreated") val dateCreated: String = "",
) {
    fun toItem(): MediaItem {
        val source = sources.firstOrNull()
        // The item answer carries the streams either in its first media source or,
        // asked with Fields=MediaStreams, at the top level; take the fuller list.
        val all = listOf(source?.streams.orEmpty(), streams).maxBy { it.size }
        val video = all.firstOrNull { it.type == "Video" }
        fun tracks(kind: String) = all.filter { it.type == kind }.map {
            // Only the extension of the path is kept: where the file lies on the server is nobody's business here.
            val format = if (it.external) it.path.substringAfterLast('.', it.codec).substringAfterLast('/').lowercase() else ""
            Track(it.index, it.codec, it.language, it.displayTitle.ifEmpty { it.language }, it.channels, it.external, format)
        }
        return MediaItem(
            id = id,
            name = name,
            type = type,
            year = year,
            overview = overview,
            runtimeSeconds = runTimeTicks / TICKS_PER_SECOND,
            resumeSeconds = userData.positionTicks / TICKS_PER_SECOND,
            played = userData.played,
            hasPoster = !imageTags["Primary"].isNullOrEmpty(),
            container = source?.container?.ifEmpty { container } ?: container,
            videoCodec = video?.codec.orEmpty(),
            width = video?.width ?: 0,
            height = video?.height ?: 0,
            sizeBytes = source?.size ?: 0,
            audio = tracks("Audio"),
            subtitles = tracks("Subtitle"),
            genres = genres,
            rating = rating,
            score = score,
            seriesId = seriesId,
            seriesName = seriesName,
            season = if (type == "Episode") parentIndex else 0,
            episode = if (type == "Episode") index else 0,
            childCount = if (recursiveCount != 0) recursiveCount else childCount,
            sourceId = source?.id?.ifEmpty { id } ?: id,
            added = dateCreated,
            favorite = userData.favorite,
        )
    }
}
