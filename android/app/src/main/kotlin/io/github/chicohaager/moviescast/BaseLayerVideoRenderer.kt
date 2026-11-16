package io.github.chicohaager.moviescast

import android.content.Context
import android.os.Handler
import android.util.Log
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.DecoderReuseEvaluation
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.FormatHolder
import androidx.media3.decoder.DecoderInputBuffer
import androidx.media3.exoplayer.Renderer
import androidx.media3.exoplayer.RendererCapabilities
import androidx.media3.exoplayer.audio.AudioSink
import io.github.chicohaager.moviescast.core.Passthrough
import androidx.media3.exoplayer.mediacodec.MediaCodecSelector
import androidx.media3.exoplayer.video.MediaCodecVideoRenderer
import androidx.media3.exoplayer.video.VideoRendererEventListener

/** The decision, in plain types so it can be tested without a device. */
object DolbyVisionBaseLayer {
    private val PROFILE_7 = Regex("^dv(he|h1)\\.07\\..*")

    /**
     * Whether a Dolby Vision track may be played as plain HEVC: only profile 7 (the UHD Blu-ray
     * profile, whose base layer is HEVC Main 10 in HDR10), only when the device cannot play the
     * track as Dolby Vision, and not when the file says its picture is something other than
     * HDR10 (PQ). Media3 itself offers no fallback for profile 7; profiles 4, 8 and 9 it handles.
     *
     * @param colorTransferIsPq true/false as the file says, null when it does not say.
     */
    fun playAsHevc(sampleMimeType: String?, codecs: String?, colorTransferIsPq: Boolean?, deviceHandlesTrack: Boolean): Boolean =
        !deviceHandlesTrack &&
            sampleMimeType == "video/dolby-vision" &&
            codecs != null && PROFILE_7.matches(codecs) &&
            colorTransferIsPq != false

    /**
     * Removes the Dolby Vision RPU (NAL unit type 62) and enhancement layer (63) from one access unit in Annex B form
     * (start codes, as Media3's Matroska and MP4 extractors hand HEVC to the decoder), in place; returns the new length.
     * What is left is the HEVC base layer alone, as the Jellyfin server sends it to a client that plays HDR10 but not
     * Dolby Vision with an enhancement layer (`EncodingHelper.cs`: `hevc_metadata=remove_dovi=1`, "Remove Dolby
     * Vision EL and RPU") and as Kodi's BitstreamConverter does when Dolby Vision is not allowed. Given to the HEVC
     * decoder with them, the Google TV Streamer played the sound and showed no picture (2026-11-16).
     */
    fun stripLayers(data: ByteArray, offset: Int, length: Int): Int {
        val end = offset + length
        fun startAt(i: Int) = i + 2 < end && data[i] == 0.toByte() && data[i + 1] == 0.toByte() && data[i + 2] == 1.toByte()
        // the starts of all start codes (3 bytes, or 4 with the zero before them)
        val units = ArrayList<IntArray>()
        var i = offset
        while (i + 2 < end) {
            if (startAt(i)) {
                val codeStart = if (i > offset && data[i - 1] == 0.toByte()) i - 1 else i
                units.add(intArrayOf(codeStart, i + 3))
                i += 3
            } else {
                i++
            }
        }
        if (units.isEmpty()) return length
        var write = offset
        // anything before the first start code stays as it is
        if (units[0][0] > offset) write = units[0][0]
        for ((n, unit) in units.withIndex()) {
            val unitEnd = if (n + 1 < units.size) units[n + 1][0] else end
            val type = if (unit[1] < end) (data[unit[1]].toInt() shr 1) and 0x3F else -1
            if (type == 62 || type == 63) continue
            val size = unitEnd - unit[0]
            if (write != unit[0]) System.arraycopy(data, unit[0], data, write, size)
            write += size
        }
        return write - offset
    }
}

/**
 * Media3's video renderer, except that a Dolby Vision profile 7 track the device has no decoder
 * for is handed to the HEVC decoder as its base layer: the viewer gets HDR10 instead of a black
 * screen (measured 2026-11-12 on a Pixel 7: no Dolby Vision decoder, HEVC Main10HDR10 up to 3840x2176).
 */
@UnstableApi
class BaseLayerVideoRenderer(builder: Builder, private val selector: MediaCodecSelector) : MediaCodecVideoRenderer(builder) {

    override fun getName(): String = "BaseLayerVideoRenderer"

    override fun supportsFormat(mediaCodecSelector: MediaCodecSelector, format: Format): Int =
        super.supportsFormat(mediaCodecSelector, baseLayer(mediaCodecSelector, format))

    override fun onInputFormatChanged(formatHolder: FormatHolder): DecoderReuseEvaluation? {
        formatHolder.format = formatHolder.format?.let { baseLayer(selector, it) }
        return super.onInputFormatChanged(formatHolder)
    }

    private fun baseLayer(mediaCodecSelector: MediaCodecSelector, format: Format): Format {
        if (format.sampleMimeType != MimeTypes.VIDEO_DOLBY_VISION) return format
        val handled = RendererCapabilities.getFormatSupport(super.supportsFormat(mediaCodecSelector, format)) == C.FORMAT_HANDLED
        val pq = format.colorInfo?.colorTransfer?.takeIf { it != Format.NO_VALUE }?.let { it == C.COLOR_TRANSFER_ST2084 }
        if (!DolbyVisionBaseLayer.playAsHevc(format.sampleMimeType, format.codecs, pq, handled)) return format
        if (!announced) {
            announced = true
            Log.i("MoviesCast", "dolby vision ${format.codecs}: no decoder, playing the HEVC base layer")
        }
        stripping = true
        return format.buildUpon().setSampleMimeType(MimeTypes.VIDEO_H265).setCodecs(null).build()
    }

    /** As Media3 strips AV1's T.35 metadata here: the access unit is rewritten in place before the decoder gets it. */
    // MediaCodecRenderer has flipped the buffer (position 0, limit = the access unit's size) and queues `limit()` bytes
    // after this call; the codec's input buffers are direct, so the unit is copied out, rewritten and put back.
    override fun onQueueInputBuffer(buffer: DecoderInputBuffer) {
        super.onQueueInputBuffer(buffer)
        val data = buffer.data ?: return
        if (!stripping || data.isReadOnly) return
        val length = data.limit()
        if (scratch.size < length) scratch = ByteArray(length)
        data.position(0)
        data.get(scratch, 0, length)
        val kept = DolbyVisionBaseLayer.stripLayers(scratch, 0, length)
        if (kept != length) {
            data.position(0)
            data.put(scratch, 0, kept)
            data.limit(kept)
        }
        data.position(0)
    }

    private var scratch = ByteArray(0)

    private var announced = false
    private var stripping = false
}

/**
 * [DefaultRenderersFactory] with [BaseLayerVideoRenderer] in place of the stock video renderer, and the audio sink
 * wrapped in [SoundSink] so the sound formats in [passthroughOff] are decoded and never handed on.
 */
@UnstableApi
class MoviesCastRenderersFactory(context: Context, private val passthroughOff: () -> Set<Passthrough.Format> = { emptySet() }) : DefaultRenderersFactory(context) {
    override fun buildAudioSink(context: Context, enableFloatOutput: Boolean, enableAudioOutputPlaybackParams: Boolean): AudioSink =
        SoundSink(super.buildAudioSink(context, enableFloatOutput, enableAudioOutputPlaybackParams)!!, passthroughOff)

    override fun buildVideoRenderers(
        context: Context,
        extensionRendererMode: Int,
        mediaCodecSelector: MediaCodecSelector,
        enableDecoderFallback: Boolean,
        eventHandler: Handler,
        eventListener: VideoRendererEventListener,
        allowedVideoJoiningTimeMs: Long,
        out: ArrayList<Renderer>,
    ) {
        val before = out.size
        super.buildVideoRenderers(context, extensionRendererMode, mediaCodecSelector, enableDecoderFallback, eventHandler, eventListener, allowedVideoJoiningTimeMs, out)
        val index = (before until out.size).firstOrNull { out[it].javaClass == MediaCodecVideoRenderer::class.java }
            ?: error("the stock video renderer is missing; Media3 changed how it builds renderers")
        out[index] = BaseLayerVideoRenderer(
            MediaCodecVideoRenderer.Builder(context)
                .setMediaCodecSelector(mediaCodecSelector)
                .setCodecAdapterFactory(codecAdapterFactory)
                .setAllowedJoiningTimeMs(allowedVideoJoiningTimeMs)
                .setEnableDecoderFallback(enableDecoderFallback)
                .setEventHandler(eventHandler)
                .setEventListener(eventListener)
                .setMaxDroppedFramesToNotify(MAX_DROPPED_VIDEO_FRAME_COUNT_TO_NOTIFY),
            mediaCodecSelector,
        )
    }
}
