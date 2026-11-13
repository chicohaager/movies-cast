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
import androidx.media3.exoplayer.Renderer
import androidx.media3.exoplayer.RendererCapabilities
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
        return format.buildUpon().setSampleMimeType(MimeTypes.VIDEO_H265).setCodecs(null).build()
    }

    private var announced = false
}

/** [DefaultRenderersFactory] with [BaseLayerVideoRenderer] in place of the stock video renderer. */
@UnstableApi
class MoviesCastRenderersFactory(context: Context) : DefaultRenderersFactory(context) {
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
