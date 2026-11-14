package io.github.chicohaager.moviescast

import android.content.Context
import android.os.SystemClock
import androidx.annotation.OptIn
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DataSource
import androidx.media3.datasource.DataSpec
import androidx.media3.datasource.TransferListener
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.inspector.MetadataRetriever
import io.github.chicohaager.moviescast.core.TechFacts
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient

/**
 * Reads what a film's file holds from its own header, as Infuse does: picture size, dynamic range, the first audio
 * track's codec and channels, frame rate. Movies names little of it for an MKV (codecs only, measured 2026-11-14).
 * Media3's MetadataRetriever runs the player's extractors without decoding or playing; only the original file
 * (`/Videos/{id}/stream?static=true`) is read, never `PlaybackInfo`, so the box converts nothing.
 */
object FileProbe {
    private const val TIMEOUT_SECONDS = 20L

    /** The facts, or null when the file could not be read in time. Logs bytes and time, never the address. */
    @OptIn(UnstableApi::class)
    suspend fun read(context: Context, http: OkHttpClient, url: String): TechFacts? = withContext(Dispatchers.IO) {
        val bytes = AtomicLong()
        val ranges = java.util.Collections.synchronizedList(ArrayList<String>())
        val counter = object : TransferListener {
            override fun onTransferInitializing(source: DataSource, dataSpec: DataSpec, isNetwork: Boolean) {
                ranges.add("${dataSpec.position}+${if (dataSpec.length == C.LENGTH_UNSET.toLong()) "*" else dataSpec.length}")
            }
            override fun onTransferStart(source: DataSource, dataSpec: DataSpec, isNetwork: Boolean) {}
            override fun onBytesTransferred(source: DataSource, dataSpec: DataSpec, isNetwork: Boolean, bytesTransferred: Int) {
                bytes.addAndGet(bytesTransferred.toLong())
            }
            override fun onTransferEnd(source: DataSource, dataSpec: DataSpec, isNetwork: Boolean) {}
        }
        val sources = DefaultMediaSourceFactory(OkHttpDataSource.Factory(http).setTransferListener(counter))
        val started = SystemClock.elapsedRealtime()
        try {
            MetadataRetriever.Builder(context, MediaItem.fromUri(url)).setMediaSourceFactory(sources).build().use { retriever ->
                val groups = retriever.retrieveTrackGroups().get(TIMEOUT_SECONDS, TimeUnit.SECONDS)
                val formats = (0 until groups.length).map { groups[it] }.flatMap { group -> (0 until group.length).map { group.getFormat(it) } }
                val facts = facts(formats)
                AppLog.i("file probe: ${bytes.get()} bytes, ${SystemClock.elapsedRealtime() - started} ms: ${facts.resolution} ${facts.hdr} ${facts.audioLabel} ${facts.frameRateLabel}")
                facts
            }
        } catch (e: Exception) {
            AppLog.w("file probe: ${causes(e)} after ${bytes.get()} bytes, ${SystemClock.elapsedRealtime() - started} ms, ranges ${ranges.takeLast(4)}")
            null
        }
    }

    /** The chain of causes by class name, with an HTTP status where there is one; no message (it can hold the address). */
    @OptIn(UnstableApi::class)
    private fun causes(e: Throwable): String = generateSequence(e) { it.cause }.take(6).joinToString(" < ") { cause ->
        val status = (cause as? androidx.media3.datasource.HttpDataSource.InvalidResponseCodeException)?.responseCode
        cause.javaClass.simpleName + (status?.let { " $it" } ?: "")
    }

    /** The first picture and the first sound track, as the extractors found them. */
    @OptIn(UnstableApi::class)
    internal fun facts(formats: List<Format>): TechFacts {
        val video = formats.firstOrNull { MimeTypes.isVideo(it.sampleMimeType) }
        val audio = formats.firstOrNull { MimeTypes.isAudio(it.sampleMimeType) }
        val hdr = when {
            video == null -> TechFacts.Hdr.UNKNOWN
            video.sampleMimeType == MimeTypes.VIDEO_DOLBY_VISION -> TechFacts.Hdr.DOLBY_VISION
            video.colorInfo?.colorTransfer == C.COLOR_TRANSFER_ST2084 -> TechFacts.Hdr.HDR10
            video.colorInfo?.colorTransfer == C.COLOR_TRANSFER_HLG -> TechFacts.Hdr.HLG
            video.colorInfo != null -> TechFacts.Hdr.SDR
            else -> TechFacts.Hdr.UNKNOWN
        }
        return TechFacts(
            width = video?.width?.takeIf { it != Format.NO_VALUE } ?: 0,
            height = video?.height?.takeIf { it != Format.NO_VALUE } ?: 0,
            hdr = hdr,
            audioCodec = audio?.sampleMimeType?.let(::codec).orEmpty(),
            channels = audio?.channelCount?.takeIf { it != Format.NO_VALUE } ?: 0,
            frameRate = video?.frameRate?.takeIf { it != Format.NO_VALUE.toFloat() && it > 0f } ?: 0f,
        )
    }

    /** The servers' name for an audio codec, from Media3's MIME type. */
    @OptIn(UnstableApi::class)
    private fun codec(mime: String): String = when (mime) {
        MimeTypes.AUDIO_TRUEHD -> "truehd"
        MimeTypes.AUDIO_E_AC3, MimeTypes.AUDIO_E_AC3_JOC -> "eac3"
        MimeTypes.AUDIO_AC3 -> "ac3"
        MimeTypes.AUDIO_DTS, MimeTypes.AUDIO_DTS_EXPRESS -> "dts"
        MimeTypes.AUDIO_DTS_HD, MimeTypes.AUDIO_DTS_X -> "dtshd"
        MimeTypes.AUDIO_AAC -> "aac"
        MimeTypes.AUDIO_FLAC -> "flac"
        MimeTypes.AUDIO_OPUS -> "opus"
        MimeTypes.AUDIO_MPEG -> "mp3"
        MimeTypes.AUDIO_RAW -> "pcm"
        else -> mime.substringAfter('/')
    }
}
